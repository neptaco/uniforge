package cmd

import (
	"github.com/spf13/cobra"
)

var editorCmd = &cobra.Command{
	Use:   "editor",
	Short: "Manage Unity Editor installations",
	Long:  `Commands for managing Unity Editor installations via Unity Hub.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if editorListCmd.RunE == nil {
			return cmd.Help()
		}
		return editorListCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(editorCmd)

	addOutputFlag(editorCmd, &editorListFormat, "Output format: text, json, tsv")
}
