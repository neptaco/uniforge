package cmd

import (
	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the Go daemon in the background",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := daemon.Start(cmd.Context(), daemonConfig(), daemon.StartOptions{
			Args: []string{"daemon", "run"},
		}); err != nil {
			return err
		}
		ui.Success("Daemon started")
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
}
