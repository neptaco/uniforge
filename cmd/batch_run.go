package cmd

import "github.com/spf13/cobra"

var batchRunCmd = &cobra.Command{
	Use:        "run [project] [-- unity-args...]",
	Short:      "Deprecated alias for run",
	RunE:       runRun,
	Deprecated: "use `uniforge run [project]` instead",
	Hidden:     true,
}

func init() {
	batchCmd.AddCommand(batchRunCmd)

	batchRunCmd.Flags().StringVar(&runLogFile, "log-file", "", "Path to save log file")
	batchRunCmd.Flags().IntVar(&runTimeout, "timeout", 3600, "Timeout in seconds")
	batchRunCmd.Flags().BoolVar(&runCIMode, "ci", false, "CI mode (optimized output format)")
	batchRunCmd.Flags().BoolVarP(&runTimestamp, "timestamp", "t", false, "Show timestamp for each line")
}
