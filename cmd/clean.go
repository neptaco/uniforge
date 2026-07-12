package cmd

import "github.com/spf13/cobra"

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean stale local Unity runtime files",
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}
