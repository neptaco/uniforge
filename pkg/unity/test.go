package unity

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/neptaco/uniforge/pkg/logger"
	"github.com/neptaco/uniforge/pkg/ui"
)

// TestSummary holds parsed test result summary
type TestSummary struct {
	Total    int
	Passed   int
	Failed   int
	Skipped  int
	Duration float64
}

// TestPlatform represents the test platform
type TestPlatform string

const (
	TestPlatformEditMode TestPlatform = "editmode"
	TestPlatformPlayMode TestPlatform = "playmode"
)

// TestConfig holds configuration for running Unity tests
type TestConfig struct {
	ProjectPath    string
	Platform       TestPlatform
	Filter         string
	ResultsFile    string
	ResultsDir     string
	LogFile        string
	TimeoutSeconds int
	CIMode         bool
	ShowTimestamp  bool
}

// TestRunner handles Unity test execution
type TestRunner struct {
	project *Project
	editor  *Editor
}

// NewTestRunner creates a new TestRunner
func NewTestRunner(project *Project) *TestRunner {
	return &TestRunner{
		project: project,
		editor:  NewEditor(project.UnityVersion),
	}
}

// RunTests executes Unity tests with the specified configuration
func (t *TestRunner) RunTests(config TestConfig) (*TestSummary, error) {
	absProjectPath, err := filepath.Abs(config.ProjectPath)
	if err != nil {
		absProjectPath = config.ProjectPath
	}

	// Check if Unity Editor is already running for this project
	if err := t.editor.CheckNotRunning(absProjectPath); err != nil {
		return nil, err
	}

	editorPath, err := t.editor.GetPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get Unity Editor path: %w", err)
	}

	preparedConfig, cleanupResults, err := prepareTestResults(config)
	if err != nil {
		return nil, err
	}
	defer cleanupResults()

	args, resultsFile := t.buildArgs(absProjectPath, preparedConfig)

	timeout := config.TimeoutSeconds
	if timeout == 0 {
		timeout = 600 // Default 10 minutes for tests
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := newUnityBatchCommand(ctx, editorPath, args...)

	log := logger.NewWithOptions(config.LogFile,
		logger.WithCIMode(config.CIMode),
		logger.WithShowTime(config.ShowTimestamp),
	)
	defer func() { _ = log.Close() }()

	cmd.Stdout = log
	cmd.Stderr = log

	projectDir := filepath.Dir(absProjectPath)
	cmd.Dir = projectDir

	ui.Debug("Running Unity tests", "path", editorPath, "args", strings.Join(args, " "))

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Unity: %w", err)
	}

	waitErr := cmd.Wait()
	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("test timeout after %d seconds", timeout)
		}
	}

	return evaluateTestOutcome(resultsFile, waitErr)
}

func prepareTestResults(config TestConfig) (TestConfig, func(), error) {
	resultsFile := config.ResultsFile
	if resultsFile == "" && config.ResultsDir != "" {
		resultsFile = filepath.Join(config.ResultsDir, fmt.Sprintf("TestResults-%s.xml", config.Platform))
	}
	if resultsFile != "" {
		if err := os.MkdirAll(filepath.Dir(resultsFile), 0o755); err != nil {
			return config, nil, fmt.Errorf("create test results directory: %w", err)
		}
		if err := os.Remove(resultsFile); err != nil && !os.IsNotExist(err) {
			return config, nil, fmt.Errorf("remove stale test results file: %w", err)
		}
		config.ResultsFile = resultsFile
		return config, func() {}, nil
	}

	tempDir, err := os.MkdirTemp("", "uniforge-test-results-*")
	if err != nil {
		return config, nil, fmt.Errorf("create temporary test results directory: %w", err)
	}

	config.ResultsFile = filepath.Join(tempDir, fmt.Sprintf("TestResults-%s.xml", config.Platform))
	return config, func() { _ = os.RemoveAll(tempDir) }, nil
}

func evaluateTestOutcome(resultsFile string, waitErr error) (*TestSummary, error) {
	summary, resultErr := parseTestResults(resultsFile)
	if waitErr != nil {
		if resultErr != nil {
			return nil, fmt.Errorf("tests failed: %w", errors.Join(waitErr, resultErr))
		}
		return summary, fmt.Errorf("tests failed: %w", waitErr)
	}
	if resultErr != nil {
		return nil, resultErr
	}
	if summary != nil && summary.Failed > 0 {
		return summary, fmt.Errorf("%d test(s) failed", summary.Failed)
	}
	return summary, nil
}

// buildArgs builds Unity CLI arguments and returns them along with the resolved results file path.
func (t *TestRunner) buildArgs(absProjectPath string, config TestConfig) ([]string, string) {
	projectName := filepath.Base(absProjectPath)

	args := []string{
		"-projectPath", projectName,
		"-batchmode",
		"-nographics",
		"-runTests",
	}

	if config.Platform != "" {
		args = append(args, "-testPlatform", string(config.Platform))
	}

	if config.Filter != "" {
		args = append(args, "-testFilter", config.Filter)
	}

	resultsFile := config.ResultsFile
	if resultsFile == "" && config.ResultsDir != "" {
		resultsFile = filepath.Join(config.ResultsDir, fmt.Sprintf("TestResults-%s.xml", config.Platform))
	}
	if resultsFile != "" {
		args = append(args, "-testResults", resultsFile)
	}

	if config.LogFile != "" {
		args = append(args, "-logFile", config.LogFile)
	} else {
		args = append(args, "-logFile", "-")
	}

	return args, resultsFile
}

// nunitTestRun represents the root element of NUnit XML test results
type nunitTestRun struct {
	XMLName  xml.Name `xml:"test-run"`
	Total    string   `xml:"total,attr"`
	Passed   string   `xml:"passed,attr"`
	Failed   string   `xml:"failed,attr"`
	Skipped  string   `xml:"skipped,attr"`
	Duration string   `xml:"duration,attr"`
}

// parseTestResults parses NUnit XML results file and returns a summary
func parseTestResults(resultsFile string) (*TestSummary, error) {
	if resultsFile == "" {
		return nil, errors.New("test results file path is empty")
	}

	data, err := os.ReadFile(resultsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read results file: %w", err)
	}

	var testRun nunitTestRun
	if err := xml.Unmarshal(data, &testRun); err != nil {
		return nil, fmt.Errorf("failed to parse results XML: %w", err)
	}

	summary := &TestSummary{
		Total:   atoi(testRun.Total),
		Passed:  atoi(testRun.Passed),
		Failed:  atoi(testRun.Failed),
		Skipped: atoi(testRun.Skipped),
	}
	summary.Duration, _ = strconv.ParseFloat(testRun.Duration, 64)

	return summary, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
