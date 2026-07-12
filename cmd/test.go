package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var (
	testPlatform   string
	testFilter     string
	testResults    string
	testResultsDir string
	testLogFile    string
	testTimeout    int
	testCIMode     bool
	testTimestamp  bool
)

var testCmd = &cobra.Command{
	Use:   "test [project]",
	Short: "Deprecated alias for batch test",
	Long: `Run Unity Test Runner with the specified configuration.

Supports both EditMode and PlayMode tests.

Examples:
  # Run all EditMode tests
  uniforge test --platform editmode

  # Run all PlayMode tests
  uniforge test --platform playmode

  # Run tests with filter (class name, partial match)
  uniforge test --platform editmode --filter MyTestClass

  # Run multiple test classes (semicolon-separated, no spaces)
  uniforge test --platform editmode --filter "ClassA;ClassB"

  # Filter by namespace (regex)
  uniforge test --platform editmode --filter "^MyNamespace\."

  # Exclude specific tests
  uniforge test --platform editmode --filter "!SlowTestClass"

  # Save test results to file
  uniforge test --platform editmode --results ./test-results.xml

  # Save test results to a directory
  uniforge test --platform editmode --results-dir /tmp/test-results/

  # CI mode with custom timeout
  uniforge test --platform editmode --ci --timeout 1800

  # Specify project path
  uniforge test /path/to/project --platform editmode`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTest,
}

func init() {
	rootCmd.AddCommand(testCmd)

	testCmd.Flags().StringVar(&testPlatform, "platform", "", "Test platform (editmode, playmode)")
	testCmd.Flags().StringVar(&testFilter, "filter", "", "Test filter (name, regex, or semicolon-separated list)")
	testCmd.Flags().StringVar(&testResults, "results", "", "Path to save test results (XML)")
	testCmd.Flags().StringVar(&testResultsDir, "results-dir", "", "Directory to save test results (XML)")
	testCmd.Flags().StringVar(&testLogFile, "log-file", "", "Path to save log file")
	testCmd.Flags().IntVar(&testTimeout, "timeout", 600, "Test timeout in seconds")
	testCmd.Flags().BoolVar(&testCIMode, "ci", false, "CI mode (optimized output format)")
	testCmd.Flags().BoolVarP(&testTimestamp, "timestamp", "t", false, "Show timestamp for each line")

	if err := testCmd.MarkFlagRequired("platform"); err != nil {
		ui.Warn("Failed to mark platform flag as required: %v", err)
	}
}

func runTest(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return err
	}

	ui.Info("Running tests for project: %s", project.Path)

	platform := unity.TestPlatform(testPlatform)
	if platform != unity.TestPlatformEditMode && platform != unity.TestPlatformPlayMode {
		return fmt.Errorf("invalid platform: %s (must be 'editmode' or 'playmode')", testPlatform)
	}

	testConfig := unity.TestConfig{
		ProjectPath:    project.Path,
		Platform:       platform,
		Filter:         testFilter,
		ResultsFile:    testResults,
		ResultsDir:     testResultsDir,
		LogFile:        testLogFile,
		TimeoutSeconds: testTimeout,
		CIMode:         testCIMode,
		ShowTimestamp:  testTimestamp,
	}

	runner := unity.NewTestRunner(project)
	summary, err := runner.RunTests(testConfig)
	if err != nil {
		if summary != nil {
			printTestSummary(summary)
		}
		return fmt.Errorf("tests failed: %w", err)
	}

	if summary != nil {
		printTestSummary(summary)
	} else {
		ui.Success("Tests completed successfully")
	}
	return nil
}

func printTestSummary(s *unity.TestSummary) {
	result := "PASSED"
	printFn := ui.Success
	if s.Failed > 0 {
		result = "FAILED"
		printFn = ui.Error
	}

	ui.Print("")
	ui.Print("=== Test Results ===")
	ui.Print("Total: %d  Passed: %d  Failed: %d  Skipped: %d", s.Total, s.Passed, s.Failed, s.Skipped)
	ui.Print("Duration: %.2fs", s.Duration)
	printFn("Result: %s", result)
}
