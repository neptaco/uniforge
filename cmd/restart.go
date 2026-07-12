package cmd

import (
	"fmt"
	"time"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var (
	restartForce   bool
	restartVersion string
)

var restartCmd = &cobra.Command{
	Use:   "restart [project]",
	Short: "Restart Unity Editor",
	Long: `Restart the Unity Editor for the specified project.
This closes the running Editor and opens it again.

Examples:
  # Restart Unity Editor for current project
  uniforge restart

  # Restart with specific project path
  uniforge restart /path/to/project

  # Force restart (SIGKILL then reopen)
  uniforge restart --force

  # Override editor version
  uniforge restart /path/to/project --version 6000.0.54f1`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRestart,
}

func init() {
	rootCmd.AddCommand(restartCmd)

	restartCmd.Flags().BoolVar(&restartForce, "force", false, "Force kill the process before restart (SIGKILL)")
	restartCmd.Flags().StringVar(&restartVersion, "version", "", "Override Unity Editor version")
}

func runRestart(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return err
	}

	version := project.UnityVersion
	if restartVersion != "" {
		version = restartVersion
	}

	editor := unity.NewEditor(version)

	// Try to close existing instance (ignore error if not running)
	_ = ui.WithSpinnerNoResult("Closing Unity Editor...", func() error {
		if err := editor.Close(project.Path, restartForce); err != nil {
			ui.Debug("No running editor found or close failed", "error", err)
		}
		return nil
	})

	// Wait a moment for the editor to fully close
	time.Sleep(2 * time.Second)

	// Open editor
	err = ui.WithSpinnerNoResult("Starting Unity Editor...", func() error {
		return editor.Open(project.Path)
	})
	if err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	ui.Success("Unity Editor %s restarted for project: %s", version, project.Name)
	return nil
}
