package cmd

import "github.com/spf13/cobra"

var batchCompileCmd = &cobra.Command{
	Use:   "compile [project]",
	Short: "Compile a Unity project in batch mode",
	RunE:  runCompile,
}

func init() {
	batchCmd.AddCommand(batchCompileCmd)

	addOutputFlag(batchCompileCmd, &compileOutput, "Output format: text, json")
	batchCompileCmd.Flags().StringVar(&compileLogFile, "log-file", "", "Path to save log file")
	batchCompileCmd.Flags().IntVar(&compileTimeout, "timeout", 300, "Compile timeout in seconds")
	batchCompileCmd.Flags().BoolVar(&compileCIMode, "ci", false, "CI mode (optimized output format)")
	batchCompileCmd.Flags().BoolVarP(&compileTimestamp, "timestamp", "t", false, "Show timestamp for each line")
}
