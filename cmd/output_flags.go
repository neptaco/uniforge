package cmd

import "github.com/spf13/cobra"

func addOutputFlag(cmd *cobra.Command, target *string, description string) {
	cmd.Flags().StringVarP(target, "output", "o", "", description)
}

func resolveOutputOrDefault(output, defaultValue string) string {
	if output == "" {
		return defaultValue
	}
	return output
}
