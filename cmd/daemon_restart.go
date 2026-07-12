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
		cfg := daemonConfig()

		// Stop handles cleanup of info file and SIGKILL escalation
		if err := daemon.Stop(cmd.Context(), cfg); err != nil {
			ui.Warn("Failed to stop daemon: %v", err)
		}

		if err := daemon.Start(cmd.Context(), cfg, daemon.StartOptions{
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
