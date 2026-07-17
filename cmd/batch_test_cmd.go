package cmd

import (
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var batchTestCmd = &cobra.Command{
	Use:        "test [project]",
	Short:      "Deprecated alias for test",
	RunE:       runTest,
	Deprecated: "use `uniforge test [project]` instead",
	Hidden:     true,
}

func init() {
	batchCmd.AddCommand(batchTestCmd)

	batchTestCmd.Flags().StringVar(&testPlatform, "platform", "", "Test platform (editmode, playmode)")
	batchTestCmd.Flags().StringVar(&testFilter, "filter", "", "Test filter (name, regex, or semicolon-separated list)")
	batchTestCmd.Flags().StringVar(&testResults, "results", "", "Path to save test results (XML)")
	batchTestCmd.Flags().StringVar(&testResultsDir, "results-dir", "", "Directory to save test results (XML)")
	batchTestCmd.Flags().StringVar(&testLogFile, "log-file", "", "Path to save log file")
	batchTestCmd.Flags().IntVar(&testTimeout, "timeout", 600, "Test timeout in seconds")
	batchTestCmd.Flags().BoolVar(&testCIMode, "ci", false, "CI mode (optimized output format)")
	batchTestCmd.Flags().BoolVarP(&testTimestamp, "timestamp", "t", false, "Show timestamp for each line")

	if err := batchTestCmd.MarkFlagRequired("platform"); err != nil {
		ui.Warn("Failed to mark platform flag as required: %v", err)
	}
}
