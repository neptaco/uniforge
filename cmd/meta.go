package cmd

import (
	"github.com/spf13/cobra"
)

var metaCmd = &cobra.Command{
	Use:   "meta",
	Short: "Manage Unity .meta files",
	Long:  `Commands for managing Unity .meta files.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if metaCheckCmd.RunE == nil {
			return cmd.Help()
		}
		return metaCheckCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(metaCmd)

	metaCmd.Flags().BoolVar(&metaCheckFix, "fix", false, "Remove orphan .meta files")
	metaCmd.Flags().BoolVar(&metaCheckForce, "force", false, "Skip confirmation when using --fix (for CI)")
}
