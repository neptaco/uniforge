package cmd

import "github.com/spf13/cobra"

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Unity batch mode commands",
}

func init() {
	rootCmd.AddCommand(batchCmd)
}
