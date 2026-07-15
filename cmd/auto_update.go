package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/neptaco/uniforge/pkg/platform"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/updater"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const backgroundUpdateCheckCommand = "__update-check"

var pendingAutoUpdateNotice *updater.AutoUpdateNotice

var backgroundUpdateCheckCmd = &cobra.Command{
	Use:    backgroundUpdateCheckCommand,
	Hidden: true,
	Args:   cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		opts, err := automaticUpdateOptions()
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		_ = updater.RefreshAutoCheck(ctx, opts)
	},
}

func init() {
	viper.SetDefault("update.check", true)
	viper.SetDefault("update.check_interval", 24*time.Hour)
	viper.SetDefault("update.notify", "auto")
	viper.SetDefault("update.remind_interval", 7*24*time.Hour)
	rootCmd.AddCommand(backgroundUpdateCheckCmd)
}

func prepareAutomaticUpdate(cmd *cobra.Command, args []string) error {
	pendingAutoUpdateNotice = nil
	if !automaticUpdateEligible(cmd) {
		return nil
	}
	opts, err := automaticUpdateOptions()
	if err != nil {
		ui.Debug("Automatic update check is unavailable", "error", err)
		return nil
	}
	decision, err := updater.PrepareAutoCheck(opts)
	if err != nil {
		ui.Debug("Failed to read automatic update cache", "error", err)
		return nil
	}
	if shouldDisplayAutomaticUpdate() {
		pendingAutoUpdateNotice = decision.Notice
	}
	if decision.CheckDue {
		startBackgroundUpdateCheck()
	}
	return nil
}

func displayAutomaticUpdate(cmd *cobra.Command, args []string) error {
	if pendingAutoUpdateNotice == nil {
		return nil
	}
	notice := pendingAutoUpdateNotice
	pendingAutoUpdateNotice = nil
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(),
		"UniForge %s is available (current %s). Run: uniforge update\n",
		notice.LatestVersion, notice.CurrentVersion); err != nil {
		return nil
	}
	if opts, err := automaticUpdateOptions(); err == nil {
		if err := updater.RecordAutoNotification(opts, notice.LatestVersion); err != nil {
			ui.Debug("Failed to record automatic update notification", "error", err)
		}
	}
	return nil
}

func automaticUpdateOptions() (updater.AutoCheckOptions, error) {
	cacheDir, err := platform.CacheDir()
	if err != nil {
		return updater.AutoCheckOptions{}, err
	}
	return updater.AutoCheckOptions{
		CurrentVersion:   Version,
		CachePath:        filepath.Join(cacheDir, "update-check.json"),
		CheckInterval:    viper.GetDuration("update.check_interval"),
		ReminderInterval: viper.GetDuration("update.remind_interval"),
	}, nil
}

func automaticUpdateEligible(cmd *cobra.Command) bool {
	if !viper.GetBool("update.check") || envPresent("UNIFORGE_NO_UPDATE_CHECK") || isCI() {
		return false
	}
	path := cmd.CommandPath()
	if path == "uniforge" || path == "uniforge "+backgroundUpdateCheckCommand {
		return false
	}
	if path == "uniforge update" || strings.HasPrefix(path, "uniforge completion") ||
		strings.HasPrefix(path, "uniforge tool") || path == "uniforge mcp serve" ||
		path == "uniforge daemon run" {
		return false
	}
	if boolFlag(cmd, "ci") || boolFlag(cmd, "path-only") || boolFlag(cmd, "count") ||
		boolFlag(cmd, "follow") || machineReadableOutput(cmd) {
		return false
	}
	return true
}

func machineReadableOutput(cmd *cobra.Command) bool {
	flag := cmd.Flags().Lookup("output")
	if flag != nil {
		format := strings.ToLower(flag.Value.String())
		if format != "" && format != "text" && format != "table" {
			return true
		}
	}
	if stdoutIsTerminal() {
		return false
	}
	switch cmd.CommandPath() {
	case "uniforge project list", "uniforge editor list", "uniforge editor available":
		return flag == nil || flag.Value.String() == ""
	default:
		return false
	}
}

func shouldDisplayAutomaticUpdate() bool {
	switch strings.ToLower(viper.GetString("update.notify")) {
	case "never":
		return false
	case "always":
		return true
	default:
		return stderrIsTerminal()
	}
}

func boolFlag(cmd *cobra.Command, name string) bool {
	flag := cmd.Flags().Lookup(name)
	return flag != nil && flag.Value.String() == "true"
}

func startBackgroundUpdateCheck() {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	child := exec.Command(executable, backgroundUpdateCheckCommand)
	configureBackgroundProcess(child)
	if err := child.Start(); err != nil {
		ui.Debug("Failed to start background update check", "error", err)
		return
	}
	_ = child.Process.Release()
}

func isCI() bool {
	for _, name := range []string{"CI", "GITHUB_ACTIONS", "BUILD_NUMBER", "TEAMCITY_VERSION"} {
		if value, ok := os.LookupEnv(name); ok && value != "" && value != "0" && !strings.EqualFold(value, "false") {
			return true
		}
	}
	return false
}

func envPresent(name string) bool {
	_, ok := os.LookupEnv(name)
	return ok
}

var stdoutIsTerminal = func() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

var stderrIsTerminal = func() bool {
	return isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
}
