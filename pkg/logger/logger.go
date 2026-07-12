package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// maxCollectedLines caps the number of error/warning lines stored in memory
const maxCollectedLines = 1000

type Logger struct {
	file             *os.File
	writer           io.Writer
	rawWriter        io.Writer // For file output without colors
	ciMode           bool
	warnings         int
	errors           int
	mutex            sync.Mutex
	pipeReader       *io.PipeReader
	pipeWriter       *io.PipeWriter
	formatter        *Formatter
	showTime         bool
	currentGroup     NoiseCategory // Current active group in CI mode
	groupIndentLevel int           // Indentation level when group started
	collectErrors    bool          // Whether to collect error/warning lines
	errorLines       []string      // Collected error lines
	warningLines     []string      // Collected warning lines
	lineObserver     func(string, LogLevel)
}

type LoggerOption func(*Logger)

func WithCIMode(ci bool) LoggerOption {
	return func(l *Logger) {
		l.ciMode = ci
	}
}

func WithFormatter(f *Formatter) LoggerOption {
	return func(l *Logger) {
		l.formatter = f
	}
}

func WithShowTime(show bool) LoggerOption {
	return func(l *Logger) {
		l.showTime = show
	}
}

func WithCollectErrors(collect bool) LoggerOption {
	return func(l *Logger) {
		l.collectErrors = collect
	}
}

func WithLineObserver(observer func(string, LogLevel)) LoggerOption {
	return func(l *Logger) {
		l.lineObserver = observer
	}
}

func New(logFile string, ciMode bool) *Logger {
	return NewWithOptions(logFile, WithCIMode(ciMode))
}

func NewWithOptions(logFile string, opts ...LoggerOption) *Logger {
	l := &Logger{
		formatter: NewFormatter(),
		showTime:  false,
	}

	for _, opt := range opts {
		opt(l)
	}

	var writers []io.Writer

	if logFile != "" && logFile != "-" {
		file, err := os.Create(logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create log file %s: %v\n", logFile, err)
		} else {
			l.file = file
			l.rawWriter = file
			writers = append(writers, file)
		}
	}

	writers = append(writers, os.Stdout)

	if len(writers) > 1 {
		l.writer = io.MultiWriter(writers...)
	} else {
		l.writer = writers[0]
	}

	l.pipeReader, l.pipeWriter = io.Pipe()

	go l.processLogs()

	return l
}

func (l *Logger) Write(p []byte) (n int, err error) {
	return l.pipeWriter.Write(p)
}

func (l *Logger) processLogs() {
	scanner := bufio.NewScanner(l.pipeReader)
	// Increase buffer for long lines
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
		l.processLine(line)
	}
}

func (l *Logger) processLine(line string) {
	level := l.formatter.ClassifyLine(line)
	if l.lineObserver != nil {
		l.lineObserver(line, level)
	}

	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Count warnings and errors (noise lines return LogLevelNoise from ClassifyLine,
	// so they never match Warning/Error here - no need for separate GetNoiseCategory call)
	switch level {
	case LogLevelWarning:
		l.warnings++
		if l.collectErrors && len(l.warningLines) < maxCollectedLines {
			l.warningLines = append(l.warningLines, line)
		}
	case LogLevelError:
		l.errors++
		if l.collectErrors && len(l.errorLines) < maxCollectedLines {
			l.errorLines = append(l.errorLines, line)
		}
	}

	// Always write raw to file
	if l.rawWriter != nil {
		_, _ = fmt.Fprintln(l.rawWriter, line)
	}

	if l.ciMode {
		noiseCategory := NoiseCategoryNone
		if level == LogLevelNoise {
			// Only compute noise category when needed for CI mode grouping.
			noiseCategory = l.formatter.GetNoiseCategory(line)
		}
		l.processLineCIMode(line, level, noiseCategory)
	} else {
		l.processLineNormalMode(line, level)
	}
}

func (l *Logger) processLineCIMode(line string, level LogLevel, noiseCategory NoiseCategory) {
	// Hide all stack traces in CI mode
	if level == LogLevelStackTrace {
		return
	}

	currentIndent := getIndentLevel(line)

	// Handle noise grouping
	if noiseCategory != NoiseCategoryNone {
		// Start a new group if category changed
		if l.currentGroup != noiseCategory {
			l.endGroup()
			l.startGroup(noiseCategory, line, currentIndent)
		} else {
			_, _ = fmt.Fprintln(os.Stdout, line)
		}
		return
	}

	// Check if this line should stay in the current group based on indentation
	// A line stays in the group if it has deeper indentation than when the group started
	if l.currentGroup != NoiseCategoryNone && currentIndent > l.groupIndentLevel {
		_, _ = fmt.Fprintln(os.Stdout, line)
		return
	}

	// Not a noise line or indentation returned to group start level - end any active group
	l.endGroup()

	// Output with annotations for errors/warnings
	switch level {
	case LogLevelError:
		_, _ = fmt.Fprintf(os.Stdout, "::error::%s\n", line)
	case LogLevelWarning:
		_, _ = fmt.Fprintf(os.Stdout, "::warning::%s\n", line)
	default:
		_, _ = fmt.Fprintln(os.Stdout, line)
	}
}

func (l *Logger) processLineNormalMode(line string, level LogLevel) {
	// Check if we should show this line
	if !l.formatter.shouldShowWithLevel(line, level) {
		return
	}

	// Format the line
	formatted := l.formatter.formatLineWithLevel(line, level)

	if l.showTime {
		timestamp := time.Now().Format("15:04:05.000")
		_, _ = fmt.Fprintf(os.Stdout, "%s[%s]%s %s\n", ColorGray, timestamp, ColorReset, formatted)
	} else {
		_, _ = fmt.Fprintln(os.Stdout, formatted)
	}
}

func (l *Logger) startGroup(category NoiseCategory, firstLine string, indentLevel int) {
	if category != NoiseCategoryNone {
		l.currentGroup = category
		l.groupIndentLevel = indentLevel
		_, _ = fmt.Fprintf(os.Stdout, "::group::%s\n", firstLine)
	}
}

func (l *Logger) endGroup() {
	if l.currentGroup != NoiseCategoryNone {
		_, _ = fmt.Fprintln(os.Stdout, "::endgroup::")
		l.currentGroup = NoiseCategoryNone
		l.groupIndentLevel = 0
	}
}

// getIndentLevel returns the indentation level of a line
// Tabs count as 1, spaces count as 1 each
func getIndentLevel(line string) int {
	level := 0
	for _, ch := range line {
		if ch == '\t' || ch == ' ' {
			level++
		} else {
			break
		}
	}
	return level
}

func (l *Logger) HasWarnings() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.warnings > 0
}

func (l *Logger) HasErrors() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.errors > 0
}

func (l *Logger) GetStats() (warnings, errors int) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.warnings, l.errors
}

func (l *Logger) GetErrorLines() []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.errorLines
}

func (l *Logger) GetWarningLines() []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.warningLines
}

func (l *Logger) Close() error {
	if l.pipeWriter != nil {
		_ = l.pipeWriter.Close()
	}

	time.Sleep(100 * time.Millisecond)

	// End any active group in CI mode
	l.mutex.Lock()
	if l.ciMode && l.currentGroup != NoiseCategoryNone {
		_, _ = fmt.Fprintln(os.Stdout, "::endgroup::")
		l.currentGroup = NoiseCategoryNone
	}
	l.mutex.Unlock()

	warnings, errors := l.GetStats()
	if warnings > 0 || errors > 0 {
		var summaryColor string
		if errors > 0 {
			summaryColor = ColorRed
		} else if warnings > 0 {
			summaryColor = ColorYellow
		}

		if l.formatter.noColor {
			summary := fmt.Sprintf("\n=== Summary: %d warnings, %d errors ===\n", warnings, errors)
			_, _ = fmt.Fprint(os.Stdout, summary)
		} else {
			summary := fmt.Sprintf("\n%s=== Summary: %d warnings, %d errors ===%s\n", summaryColor, warnings, errors, ColorReset)
			_, _ = fmt.Fprint(os.Stdout, summary)
		}
	}

	if l.file != nil {
		return l.file.Close()
	}

	return nil
}
