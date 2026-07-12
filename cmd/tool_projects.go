package cmd

import (
	"github.com/spf13/cobra"
)

var (
	toolProjectsOutput          string
	toolProjectsTimeoutMS       int
	toolProjectsAutoStartDaemon bool
)

var toolProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "List Unity projects currently connected to the daemon",
	RunE:  runToolProjects,
}

func init() {
	toolCmd.AddCommand(toolProjectsCmd)

	toolProjectsCmd.Flags().StringVarP(&toolProjectsOutput, "output", "o", "yaml", "Output format: yaml, json")
	toolProjectsCmd.Flags().IntVar(&toolProjectsTimeoutMS, "timeout", 30000, "Request timeout in milliseconds")
	toolProjectsCmd.Flags().BoolVar(&toolProjectsAutoStartDaemon, "auto-start-daemon", true, "Start the daemon automatically if needed")
}

type toolProjectEntry struct {
	ID        string   `json:"id" yaml:"id"`
	Name      string   `json:"name" yaml:"name"`
	GitRoot   string   `json:"gitRoot,omitempty" yaml:"gitRoot,omitempty"`
	Connected bool     `json:"connected" yaml:"connected"`
	Tools     []string `json:"tools,omitempty" yaml:"tools,omitempty"`
}

func runToolProjects(cmd *cobra.Command, args []string) error {
	client := newToolClient(toolClientOptions{
		output:          toolProjectsOutput,
		timeoutMS:       toolProjectsTimeoutMS,
		autoStartDaemon: toolProjectsAutoStartDaemon,
	})
	defer func() { _ = client.Close() }()

	if err := client.Connect(); err != nil {
		return err
	}
	if _, err := client.Register(); err != nil {
		return err
	}

	projectsResult, err := client.ListProjects(true)
	if err != nil {
		return err
	}

	entries := make([]toolProjectEntry, 0, len(projectsResult.Projects))
	for _, project := range projectsResult.Projects {
		entry := toolProjectEntry{
			ID:        project.ID,
			Name:      project.Name,
			GitRoot:   project.GitRoot,
			Connected: project.Connected,
		}
		for _, tool := range project.Tools {
			entry.Tools = append(entry.Tools, tool.Name)
		}
		entries = append(entries, entry)
	}

	return writeStructuredOutput(toolProjectsOutput, entries)
}
