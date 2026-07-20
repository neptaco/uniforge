package cmd

import (
	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the Go daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemon.Restart(cmd.Context(), daemonConfig(), daemon.StartOptions{
			Args: []string{"daemon", "run"},
		}); err != nil {
			return err
		}

		ui.Success("Daemon restarted")
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonRestartCmd)
}
