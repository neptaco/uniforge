package cmd

import (
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage UniForge cache",
	Long:  `Commands to manage the UniForge cache for Unity releases.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if cacheClearCmd.RunE == nil {
			return cmd.Help()
		}
		return cacheClearCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(cacheCmd)
}
