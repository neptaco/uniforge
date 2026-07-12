package cmd

import (
	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Go daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemon.Stop(cmd.Context(), daemonConfig()); err != nil {
			return err
		}
		ui.Success("Daemon stopped")
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonStopCmd)
}
