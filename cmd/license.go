package cmd

import (
	"github.com/spf13/cobra"
)

var licenseCmd = &cobra.Command{
	Use:   "license",
	Short: "Manage Unity license",
	Long:  `Commands for managing Unity license activation and return.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if licenseStatusCmd.RunE == nil {
			return cmd.Help()
		}
		return licenseStatusCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(licenseCmd)
}
