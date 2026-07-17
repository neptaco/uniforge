package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var projectOpenCmd = &cobra.Command{
	Use:   "open [project]",
	Short: "Deprecated alias for open",
	Long: `Open a Unity Hub project in Unity Editor.

The project can be specified by name (partial match) or index (1-based).
The appropriate Unity Editor version is automatically detected from the project.

Examples:
  # Open by project name
  uniforge project open my-project

  # Open by partial name
  uniforge project open guitar

  # Open by index
  uniforge project open 1

  # Interactive selection
  uniforge project open`,
	Args:       cobra.MaximumNArgs(1),
	RunE:       runProjectOpen,
	Deprecated: "use `uniforge open [project]` instead",
	Hidden:     true,
}

func init() {
	projectCmd.AddCommand(projectOpenCmd)
}

func runProjectOpen(cmd *cobra.Command, args []string) error {
	project, err := resolveHubProjectArg(args)
	if err != nil {
		return fmt.Errorf("failed to find project: %w", err)
	}

	ui.Info("Opening project: %s (%s)", project.Title, project.Version)

	err = ui.WithSpinnerNoResult("Starting Unity Editor...", func() error {
		editor := unity.NewEditor(project.Version)
		return editor.Open(project.Path)
	})
	if err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	ui.Success("Unity Editor %s started for project: %s", project.Version, project.Title)
	return nil
}
