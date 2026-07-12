package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
	"github.com/spf13/cobra"
)

var (
	openVersion string
)

var openCmd = &cobra.Command{
	Use:   "open [project]",
	Short: "Open Unity Editor with a project",
	Long: `Open Unity Editor with the specified project in GUI mode.
The Editor version is automatically detected from the project's ProjectVersion.txt.

If the argument is not a valid project path, it will search Unity Hub's
registered projects by name.

Examples:
  # Open current directory as Unity project
  uniforge open

  # Open a specific project by path
  uniforge open /path/to/project

  # Open a project by name (searches Unity Hub projects)
  uniforge open my-project

  # Override editor version
  uniforge open my-project --version 6000.0.54f1`,
	Args: cobra.MaximumNArgs(1),
	RunE: runOpen,
}

func init() {
	rootCmd.AddCommand(openCmd)

	openCmd.Flags().StringVar(&openVersion, "version", "", "Override Unity Editor version")
}

func runOpen(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return err
	}

	version := project.UnityVersion
	if openVersion != "" {
		version = openVersion
	}

	return openProject(project.Path, version, project.Name)
}

func openProject(path, version, name string) error {
	err := ui.WithSpinnerNoResult("Starting Unity Editor...", func() error {
		editor := unity.NewEditor(version)
		return editor.Open(path)
	})
	if err != nil {
		return fmt.Errorf("failed to open editor: %w", err)
	}

	ui.Success("Unity Editor %s started for project: %s", version, name)
	return nil
}
