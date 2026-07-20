package cmd

import (
	"encoding/json"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/spf13/cobra"
)

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Go daemon in the foreground",
	RunE: func(cmd *cobra.Command, args []string) error {
		meta, _ := json.Marshal(newDaemonMeta(Version))
		server := bridge.NewServer()
		return daemon.RunDaemon(cmd.Context(), daemonConfig(), daemon.RunOptions{
			Meta:  meta,
			Serve: server.Serve,
			OnShutdown: func() {
				_ = server.Stop()
			},
		})
	},
}

func newDaemonMeta(version string) bridge.DaemonMeta {
	return bridge.DaemonMeta{
		ProtocolVersion: bridge.ProtocolVersion,
		Version:         version,
	}
}

func init() {
	daemonCmd.AddCommand(daemonRunCmd)
}
