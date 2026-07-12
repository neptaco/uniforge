package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/neptaco/uniforge/pkg/daemon"
	"github.com/neptaco/uniforge/pkg/updater"
	"github.com/spf13/cobra"
)

var (
	updateCheck   bool
	updateVersion string
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update UniForge to the latest release",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
		defer cancel()
		cfg := daemonConfig()
		restartDaemon := !updateCheck && daemon.IsRunning(cfg)
		if restartDaemon {
			if err := daemon.Stop(ctx, cfg); err != nil {
				return fmt.Errorf("stop daemon before update: %w", err)
			}
		}
		result, err := updater.Run(ctx, updater.Options{
			CurrentVersion: Version,
			Version:        updateVersion,
			CheckOnly:      updateCheck,
		})
		if err != nil {
			if restartDaemon {
				_ = daemon.Start(context.Background(), cfg, daemon.StartOptions{Args: []string{"daemon", "run"}})
			}
			return err
		}
		if restartDaemon {
			if err := daemon.Start(ctx, cfg, daemon.StartOptions{Args: []string{"daemon", "run"}}); err != nil {
				return fmt.Errorf("restart daemon after update: %w", err)
			}
		}
		if updateCheck {
			if result.CurrentVersion == result.TargetVersion || "v"+result.CurrentVersion == result.TargetVersion {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "UniForge is up to date (%s)\n", result.CurrentVersion)
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Update available: %s -> %s\n", result.CurrentVersion, result.TargetVersion)
			}
			return err
		}
		if !result.Updated {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "UniForge is already up to date (%s)\n", result.CurrentVersion)
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Updated UniForge: %s -> %s\n", result.CurrentVersion, result.TargetVersion)
		return err
	},
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "check for an available update without installing it")
	updateCmd.Flags().StringVar(&updateVersion, "version", "latest", "release version to install (vX.Y.Z or latest)")
	rootCmd.AddCommand(updateCmd)
}
