package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var projectPathCmd = &cobra.Command{
	Use:   "path [project]",
	Short: "Print project path",
	Long: `Print the filesystem path of a project.

The project defaults to the current directory and may be specified by path,
Unity Hub project name (partial match), or Unity Hub project index (1-based).

Examples:
  # Get path by project name
  uniforge project path my-project

  # Get path by index
  uniforge project path 1

  # Get the current project path
  uniforge project path

  # Use in shell commands
  cd $(uniforge project path my-project)
  code $(uniforge project path my-project)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProjectPath,
}

func init() {
	projectCmd.AddCommand(projectPathCmd)
}

func runProjectPath(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return fmt.Errorf("failed to resolve project: %w", err)
	}

	fmt.Println(project.Path)
	return nil
}
