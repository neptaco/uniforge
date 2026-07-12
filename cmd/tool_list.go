package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/spf13/cobra"
)

var (
	toolListProject         string
	toolListFormat          string
	toolListTimeoutMS       int
	toolListAutoStartDaemon bool
	toolListVerbose         bool
)

var toolListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Unity tools exposed by connected project",
	Long: `List Unity tools exposed by the connected project.

Tools are resolved from the project matching the current directory or --project flag.`,
	RunE: runToolList,
}

const toolListDescribeHint = "AI agents: This list omits schema details to save context. Use `uniforge tool describe <name>` when you need full tool details."

func init() {
	toolCmd.AddCommand(toolListCmd)

	toolListCmd.Flags().StringVarP(&toolListProject, "project", "p", "", "Project ID or name")
	toolListCmd.Flags().StringVarP(&toolListFormat, "output", "o", "yaml", "Output format: yaml, json")
	toolListCmd.Flags().IntVar(&toolListTimeoutMS, "timeout", 30000, "Request timeout in milliseconds")
	toolListCmd.Flags().BoolVar(&toolListAutoStartDaemon, "auto-start-daemon", true, "Start the daemon automatically if needed")
	toolListCmd.Flags().BoolVarP(&toolListVerbose, "verbose", "v", false, "Show daemon and connection logs")
}

type toolEntry struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
}

type toolListResult struct {
	Project    string      `json:"project" yaml:"project"`
	SchemaHash string      `json:"schema_hash" yaml:"schema_hash"`
	Tools      []toolEntry `json:"tools" yaml:"tools"`
}

func runToolList(cmd *cobra.Command, args []string) error {
	client := newToolClient(toolClientOptions{
		project:         toolListProject,
		output:          toolListFormat,
		timeoutMS:       toolListTimeoutMS,
		autoStartDaemon: toolListAutoStartDaemon,
	})
	defer func() { _ = client.Close() }()

	writeToolVerbose(toolListVerbose, "Connecting to daemon")
	if err := client.Connect(); err != nil {
		return err
	}
	writeToolVerbose(toolListVerbose, "Registering tool client")
	if _, err := client.Register(); err != nil {
		return err
	}

	writeToolVerbose(toolListVerbose, "Fetching connected Unity projects")
	projectsResult, err := client.ListProjects(true)
	if err != nil {
		return err
	}

	project, err := bridge.ResolveProject(toolListProject, bridge.CwdHints{}, projectsResult.Projects)
	if err != nil {
		return err
	}

	return writeToolList(toolListFormat, buildToolList(*project))
}

func buildToolList(project bridge.ProjectInfo) toolListResult {
	entries := make([]toolEntry, 0, len(project.Tools))
	for _, tool := range project.Tools {
		entries = append(entries, toolEntry{
			Name:        tool.Name,
			Description: tool.Description,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return toolListResult{
		Project:    project.Name,
		SchemaHash: project.SchemaHash,
		Tools:      entries,
	}
}

func writeToolList(format string, result toolListResult) error {
	stdout, stderr, err := renderToolList(format, result)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(os.Stdout, stdout); err != nil {
		return err
	}
	if stderr == "" {
		return nil
	}
	_, err = fmt.Fprintln(os.Stderr, stderr)
	return err
}

func renderToolList(format string, result toolListResult) (string, string, error) {
	switch format {
	case "json":
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", "", err
		}
		return string(data) + "\n", "", nil
	case "yaml":
		projectStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75"))
		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
		descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
		hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

		var sb strings.Builder
		sb.WriteString("project: ")
		sb.WriteString(projectStyle.Render(result.Project))
		sb.WriteString("\nschema_hash: ")
		sb.WriteString(result.SchemaHash)
		sb.WriteString("\ntools:\n")
		for _, t := range result.Tools {
			sb.WriteString("  ")
			sb.WriteString(nameStyle.Render(t.Name))
			sb.WriteString(": ")
			sb.WriteString(descStyle.Render(t.Description))
			sb.WriteString("\n")
		}
		sb.WriteString(hintStyle.Render("# " + toolListDescribeHint))
		sb.WriteString("\n")
		return sb.String(), "", nil
	default:
		return "", "", fmt.Errorf("unsupported output format: %s", format)
	}
}
