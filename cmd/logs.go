package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/logger"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	logFollow    bool
	logEditor    bool
	logLines     int
	logRaw       bool
	logFilter    string
	logTrace     bool
	logFullTrace bool
	logTimestamp bool
)

var logCmd = &cobra.Command{
	Use:   "logs [project]",
	Short: "Display Unity Editor log",
	Long: `Display the Unity Editor log file with syntax highlighting.

The log file location is platform-specific:
  - macOS: ~/Library/Logs/Unity/Editor.log
  - Windows: %LOCALAPPDATA%\Unity\Editor\Editor.log
  - Linux: ~/.config/unity3d/Editor.log

Log lines are colorized:
  - Red: Errors and exceptions
  - Yellow: Warnings
  - Gray: Stack traces and startup noise

Examples:
  # Show last 100 lines (default)
  uniforge logs

  # Show logs for a connected project
  uniforge logs /path/to/project

  # Show last 500 lines
  uniforge logs -n 500

  # Filter lines by text
  uniforge logs --filter compiler

  # Follow log in real-time (like tail -f)
  uniforge logs -f

  # Follow with timestamps
  uniforge logs -f -t

  # Show raw output without colors
  uniforge logs --raw

  # Show project stack traces (Assets/, Packages/)
  uniforge logs --trace

  # Show full stack traces (including Unity internals)
  uniforge logs --full-trace

  # Open in text editor
  uniforge logs --editor`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLog,
}

func init() {
	rootCmd.AddCommand(logCmd)

	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "Follow log output in real-time")
	logCmd.Flags().BoolVar(&logEditor, "editor", false, "Open log in text editor ($EDITOR or vim)")
	logCmd.Flags().IntVarP(&logLines, "lines", "n", 100, "Number of lines to show")
	logCmd.Flags().StringVar(&logFilter, "filter", "", "Case-insensitive filter string")
	logCmd.Flags().BoolVar(&logRaw, "raw", false, "Show raw output without colors or filtering")
	logCmd.Flags().BoolVar(&logTrace, "trace", false, "Show project stack traces (Assets/, Packages/)")
	logCmd.Flags().BoolVar(&logFullTrace, "full-trace", false, "Show full stack traces including Unity internals")
	logCmd.Flags().BoolVarP(&logTimestamp, "timestamp", "t", false, "Show timestamp for each line")
}

func runLog(cmd *cobra.Command, args []string) error {
	projects := connectedProjectsForLogs()
	cwd, _ := os.Getwd()
	projectArg := ""
	if len(args) > 0 {
		projectArg = args[0]
	}

	fallback, fallbackErr := unity.GetEditorLogPath()
	logPath, note, err := resolveEditorLogTarget(
		projects,
		projectArg,
		cwd,
		fallback,
		findExistingManagedEditorLog,
	)
	if err != nil {
		return err
	}
	if logPath == "" {
		return fmt.Errorf("failed to get log path: %w", fallbackErr)
	}
	if note != "" && !logRaw {
		ui.Note("%s", note)
	}

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return fmt.Errorf("log file not found: %s", logPath)
	}

	ui.Debug("Log file path", "path", logPath)

	if logEditor {
		return openInEditor(logPath)
	}

	if logFollow {
		return followLog(logPath)
	}

	return showLog(logPath, logLines)
}

func connectedProjectsForLogs() []bridge.ProjectInfo {
	client := newToolClient(toolClientOptions{autoStartDaemon: false, timeoutMS: 2000})
	if err := client.Connect(); err != nil {
		return nil
	}
	defer func() { _ = client.Close() }()

	if _, err := client.Register(); err != nil {
		return nil
	}
	result, err := client.ListProjects(false)
	if err != nil {
		return nil
	}
	return result.Projects
}

type managedEditorLogLookup func(projectPath string) (path string, exists bool)

func resolveEditorLogTarget(
	projects []bridge.ProjectInfo,
	projectArg, cwd, fallback string,
	managedLog managedEditorLogLookup,
) (path string, note string, err error) {
	selected, offlineProjectPath, err := bridge.ResolveProjectOrPath(projectArg, cwd, projects)
	if err != nil {
		return "", "", err
	}

	if selected != nil && selected.ConsoleLogPath != "" {
		return selected.ConsoleLogPath, fmt.Sprintf(
			"Reading log for connected project %s: %s",
			selected.Name,
			selected.ConsoleLogPath,
		), nil
	}

	if selected == nil && offlineProjectPath != "" && managedLog != nil {
		if managedPath, exists := managedLog(offlineProjectPath); exists {
			projectName := filepath.Base(filepath.Clean(offlineProjectPath))
			return managedPath, fmt.Sprintf(
				"Reading managed log for project %s: %s",
				projectName,
				managedPath,
			), nil
		}
	}

	if fallback == "" {
		return "", "", nil
	}
	return fallback, fmt.Sprintf("Reading global Unity Editor log: %s", fallback), nil
}

func findExistingManagedEditorLog(projectPath string) (string, bool) {
	managedPath, err := unity.GetManagedEditorLogPath(projectPath)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(managedPath)
	return managedPath, err == nil && !info.IsDir()
}

func openInEditor(logPath string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	cmd := exec.Command(editor, logPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func followLog(logPath string) error {
	noColor := viper.GetBool("no-color") || os.Getenv("NO_COLOR") != ""

	fmt.Printf("Following %s (Ctrl+C to stop)\n\n", logPath)

	var formatter *logger.Formatter
	if !logRaw && !noColor {
		formatter = logger.NewFormatter(
			logger.WithNoColor(false),
			logger.WithHideStackTrace(!logFullTrace),
			logger.WithHideAllStackTraces(!logTrace && !logFullTrace),
		)
	}

	// Set up signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}
	defer func() { _ = watcher.Close() }()

	// Watch the directory (to detect file recreation)
	dir := logPath[:len(logPath)-len("/"+logPath[len(logPath)-len("Editor.log"):])]
	if idx := lastIndexOfPathSeparator(logPath); idx >= 0 {
		dir = logPath[:idx]
	}
	if err := watcher.Add(dir); err != nil {
		ui.Debug("Failed to watch directory, falling back to file-only watch", "error", err)
	}

	// Also watch the file itself
	if err := watcher.Add(logPath); err != nil {
		return fmt.Errorf("failed to watch log file: %w", err)
	}

	// Open file and seek to end
	file, offset, err := openAndSeekToEnd(logPath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	// Create a ticker for polling (as backup for platforms where fsnotify may not work perfectly)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			fmt.Println("\nStopped following log.")
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}

			// Handle file write or create (file recreation)
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// If file was recreated, reopen it
				if event.Has(fsnotify.Create) && event.Name == logPath {
					_ = file.Close()
					time.Sleep(100 * time.Millisecond) // Wait for file to be ready
					file, offset, err = openAndSeekToEnd(logPath)
					if err != nil {
						ui.Debug("Failed to reopen file", "error", err)
						continue
					}
					offset = 0 // Start from beginning of new file
				}

				offset, err = readNewLines(file, offset, formatter)
				if err != nil {
					ui.Debug("Error reading new lines", "error", err)
				}
			}

		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			ui.Debug("Watcher error", "error", watchErr)

		case <-ticker.C:
			// Periodic poll as backup
			offset, err = readNewLines(file, offset, formatter)
			if err != nil {
				// File might have been recreated
				if _, statErr := os.Stat(logPath); statErr == nil {
					_ = file.Close()
					file, _, err = openAndSeekToEnd(logPath)
					if err != nil {
						ui.Debug("Failed to reopen file", "error", err)
					}
					offset = 0
				}
			}
		}
	}
}

// lastIndexOfPathSeparator returns the index of the last path separator in the path
func lastIndexOfPathSeparator(path string) int {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return i
		}
	}
	return -1
}

// openAndSeekToEnd opens a file and seeks to the end, returning the file and its size
func openAndSeekToEnd(path string) (*os.File, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open log file: %w", err)
	}

	// Seek to end
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		_ = file.Close()
		return nil, 0, fmt.Errorf("failed to seek to end: %w", err)
	}

	return file, offset, nil
}

// readNewLines reads new lines from the file starting at offset
func readNewLines(file *os.File, offset int64, formatter *logger.Formatter) (int64, error) {
	// Get current file size
	info, err := file.Stat()
	if err != nil {
		return offset, err
	}

	// If file was truncated, start from beginning
	if info.Size() < offset {
		offset = 0
	}

	// If no new content, return
	if info.Size() == offset {
		return offset, nil
	}

	// Seek to last position
	_, err = file.Seek(offset, io.SeekStart)
	if err != nil {
		return offset, err
	}

	// Read new content
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// Partial line, put it back by adjusting offset
				break
			}
			return offset, err
		}

		// Update offset
		offset += int64(len(line))

		// Remove trailing newline/carriage return
		line = trimLineEnding(line)

		if !matchesLogFilter(line) {
			continue
		}

		// Output the line
		if formatter != nil {
			if formatter.ShouldShow(line) {
				formatted := formatter.FormatLine(line)
				if logTimestamp {
					ts := time.Now().Format("15:04:05.000")
					fmt.Printf("%s[%s]%s %s\n", logger.ColorGray, ts, logger.ColorReset, formatted)
				} else {
					fmt.Println(formatted)
				}
			}
		} else {
			// Raw output
			fmt.Println(line)
		}
	}

	return offset, nil
}

// trimLineEnding removes \n and \r\n from the end of a line
func trimLineEnding(line string) string {
	line = line[:len(line)-1] // Remove \n
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1] // Remove \r if present (Windows)
	}
	return line
}

func showLog(logPath string, lines int) error {
	file, err := os.Open(logPath)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Read all lines into a buffer
	var allLines []string
	scanner := bufio.NewScanner(file)

	// Increase buffer size for long lines
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read log file: %w", err)
	}

	// Calculate starting position
	start := len(allLines) - lines
	if start < 0 {
		start = 0
	}

	noColor := viper.GetBool("no-color") || os.Getenv("NO_COLOR") != ""

	if logRaw || noColor {
		// Print raw without formatting
		for i := start; i < len(allLines); i++ {
			if !matchesLogFilter(allLines[i]) {
				continue
			}
			fmt.Println(allLines[i])
		}
		return nil
	}

	// Print with formatting
	formatter := logger.NewFormatter(
		logger.WithNoColor(false),
		logger.WithHideStackTrace(!logFullTrace),
		logger.WithHideAllStackTraces(!logTrace && !logFullTrace),
	)

	for i := start; i < len(allLines); i++ {
		line := allLines[i]
		if !matchesLogFilter(line) {
			continue
		}
		if formatter.ShouldShow(line) {
			formatted := formatter.FormatLine(line)
			if logTimestamp {
				// For historical logs, show line number instead of time
				fmt.Printf("%s[%5d]%s %s\n", logger.ColorGray, i+1, logger.ColorReset, formatted)
			} else {
				fmt.Println(formatted)
			}
		}
	}

	return nil
}

func matchesLogFilter(line string) bool {
	if strings.TrimSpace(logFilter) == "" {
		return true
	}
	return strings.Contains(strings.ToLower(line), strings.ToLower(logFilter))
}
