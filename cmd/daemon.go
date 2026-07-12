package cmd

import (
	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/spf13/cobra"
)

const daemonAppName = "uniforge"

// daemonConfig returns the daemon.Config used by all daemon subcommands.
func daemonConfig() daemon.Config {
	return daemon.Config{Name: daemonAppName}
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the Go-based UniForge daemon",
	Long:  `Manage the Go-based UniForge daemon used for Unity tool execution.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if daemonStatusCmd.RunE == nil {
			return cmd.Help()
		}
		return daemonStatusCmd.RunE(cmd, args)
	},
}

func init() {
	rootCmd.AddCommand(daemonCmd)
}
