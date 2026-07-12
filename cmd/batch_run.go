package cmd

import "github.com/spf13/cobra"

var batchRunCmd = &cobra.Command{
	Use:   "run [project] [-- unity-args...]",
	Short: "Run Unity in batch mode with custom arguments",
	RunE:  runRun,
}

func init() {
	batchCmd.AddCommand(batchRunCmd)

	batchRunCmd.Flags().StringVar(&runLogFile, "log-file", "", "Path to save log file")
	batchRunCmd.Flags().IntVar(&runTimeout, "timeout", 3600, "Timeout in seconds")
	batchRunCmd.Flags().BoolVar(&runCIMode, "ci", false, "CI mode (optimized output format)")
	batchRunCmd.Flags().BoolVarP(&runTimestamp, "timestamp", "t", false, "Show timestamp for each line")
}
