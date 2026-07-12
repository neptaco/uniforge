package cmd

import "github.com/spf13/cobra"

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose local UniForge and Unity state",
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
