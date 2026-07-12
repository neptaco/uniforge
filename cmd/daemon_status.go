package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Go daemon status",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := daemonConfig()

		if !daemon.IsRunning(cfg) {
			ui.Info("Daemon is not running")
			return nil
		}

		info, err := daemon.ReadInfo(cfg)
		if err != nil {
			return err
		}

		ui.Success("Daemon is running")
		w := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(w, "  pid: %d\n", info.PID)
		_, _ = fmt.Fprintf(w, "  transport: %s\n", info.Transport)
		if info.Endpoint != "" {
			_, _ = fmt.Fprintf(w, "  endpoint: %s\n", info.Endpoint)
		}
		if info.Host != "" || info.Port != 0 {
			_, _ = fmt.Fprintf(w, "  address: %s:%d\n", info.Host, info.Port)
		}

		// Display bridge-specific metadata if present
		if len(info.Metadata) > 0 {
			var meta bridge.DaemonMeta
			if json.Unmarshal(info.Metadata, &meta) == nil {
				if meta.ProtocolVersion > 0 {
					_, _ = fmt.Fprintf(w, "  protocolVersion: %d\n", meta.ProtocolVersion)
				}
				if meta.Version != "" {
					_, _ = fmt.Fprintf(w, "  version: %s\n", meta.Version)
				}
			}
		}
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonStatusCmd)
}
