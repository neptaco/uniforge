package cmd

import "github.com/spf13/cobra"

var cleanCmd = &cobra.Command{
	Use:   "clean [project]",
	Short: "Clean selected stale Unity project runtime files",
	Long: `Clean explicitly selected stale runtime files for a Unity project.

The project defaults to the current directory and may also be specified by
path, Unity Hub project name, or Unity Hub project index. The command verifies
that the Unity Editor is not running before removing project runtime files.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
	addCleanFlags(cleanCmd)
}
