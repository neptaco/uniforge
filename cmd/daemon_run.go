package cmd

import (
	"encoding/json"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Go daemon in the foreground",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := daemonConfig()

		// Stop any existing daemon (handles "no daemon" case gracefully)
		if err := daemon.Stop(cmd.Context(), cfg); err != nil {
			ui.Warn("Failed to stop existing daemon: %v", err)
		}

		d := daemon.New(cfg)
		if err := d.Lock(); err != nil {
			return err
		}

		meta, _ := json.Marshal(newDaemonMeta(Version))
		listener, err := d.Listen(meta)
		if err != nil {
			_ = d.Shutdown()
			return err
		}

		server := bridge.NewServer()

		var stopOnce sync.Once
		shutdown := func() {
			stopOnce.Do(func() {
				_ = server.Stop()
				_ = d.Shutdown()
			})
		}
		defer shutdown()

		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-signals
			shutdown()
		}()

		return server.Serve(listener)
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
