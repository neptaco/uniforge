package cmd

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/neptaco/uniforge/pkg/platform"
	"github.com/neptaco/uniforge/pkg/updater"
	"github.com/spf13/cobra"
)

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the Go daemon in the foreground",
	RunE: func(cmd *cobra.Command, args []string) error {
		meta, _ := json.Marshal(newDaemonMeta(Version))
		server := bridge.NewServer()
		if opts, err := unityPackageAutoCheckOptions(); err == nil {
			server = bridge.NewServer(bridge.WithLatestUnityPackageVersionProvider(newUnityPackageVersionProvider(opts)))
		}
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

func unityPackageAutoCheckOptions() (updater.AutoCheckOptions, error) {
	cacheDir, err := platform.CacheDir()
	if err != nil {
		return updater.AutoCheckOptions{}, err
	}
	return updater.AutoCheckOptions{
		CachePath: filepath.Join(cacheDir, updater.UnityPackageUpdateCacheFilename),
	}, nil
}

func newUnityPackageVersionProvider(opts updater.AutoCheckOptions) func() string {
	return func() string {
		decision, err := updater.PrepareUnityPackageAutoCheck(opts)
		if err != nil {
			return ""
		}
		if decision.CheckDue {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = updater.RefreshUnityPackageAutoCheck(ctx, opts)
			}()
		}
		return decision.LatestVersion
	}
}

func init() {
	daemonCmd.AddCommand(daemonRunCmd)
}
