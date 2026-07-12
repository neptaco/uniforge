package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/tools"
	"github.com/spf13/cobra"
)

var (
	toolDescribeProject         string
	toolDescribeOutput          string
	toolDescribeTimeoutMS       int
	toolDescribeAutoStartDaemon bool
	toolDescribeVerbose         bool
)

var toolDescribeCmd = &cobra.Command{
	Use:   "describe <name> [name...]",
	Short: "Show tool schema details",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runToolDescribe,
}

func init() {
	toolCmd.AddCommand(toolDescribeCmd)

	toolDescribeCmd.Flags().StringVarP(&toolDescribeProject, "project", "p", "", "Project ID or name")
	toolDescribeCmd.Flags().StringVarP(&toolDescribeOutput, "output", "o", "yaml", "Output format: yaml, json")
	toolDescribeCmd.Flags().IntVar(&toolDescribeTimeoutMS, "timeout", 30000, "Request timeout in milliseconds")
	toolDescribeCmd.Flags().BoolVar(&toolDescribeAutoStartDaemon, "auto-start-daemon", true, "Start the daemon automatically if needed")
	toolDescribeCmd.Flags().BoolVarP(&toolDescribeVerbose, "verbose", "v", false, "Show daemon and connection logs")
}

type toolDescribePayload struct {
	Name           string         `json:"name" yaml:"name"`
	Description    string         `json:"description" yaml:"description"`
	Annotations    map[string]any `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	ExamplePayload any            `json:"examplePayload" yaml:"examplePayload"`
	InputSchema    map[string]any `json:"inputSchema" yaml:"inputSchema"`
	OutputSchema   map[string]any `json:"outputSchema,omitempty" yaml:"outputSchema,omitempty"`
}

func runToolDescribe(cmd *cobra.Command, args []string) error {
	// base tools だけで済むかチェック
	var needDaemon bool
	for _, name := range args {
		if _, ok := tools.FindBaseDefinition(name); !ok {
			needDaemon = true
			break
		}
	}

	var dynamicProjects []bridge.ProjectInfo
	if needDaemon {
		client := newToolClient(toolClientOptions{
			project:         toolDescribeProject,
			output:          toolDescribeOutput,
			timeoutMS:       toolDescribeTimeoutMS,
			autoStartDaemon: toolDescribeAutoStartDaemon,
		})
		defer func() { _ = client.Close() }()

		writeToolVerbose(toolDescribeVerbose, "Connecting to daemon")
		if err := client.Connect(); err != nil {
			return err
		}
		writeToolVerbose(toolDescribeVerbose, "Registering tool client")
		if _, err := client.Register(); err != nil {
			return err
		}

		projectsResult, err := client.ListProjects(true)
		if err != nil {
			return err
		}
		dynamicProjects = projectsResult.Projects
	}

	// 単一ツール: そのまま出力
	if len(args) == 1 {
		payload, err := resolveDescribePayload(args[0], dynamicProjects)
		if err != nil {
			return err
		}
		return writeStructuredOutput(toolDescribeOutput, payload)
	}

	// 複数ツール: 配列として出力
	payloads := make([]toolDescribePayload, 0, len(args))
	for _, name := range args {
		payload, err := resolveDescribePayload(name, dynamicProjects)
		if err != nil {
			return err
		}
		payloads = append(payloads, payload)
	}
	return writeStructuredOutput(toolDescribeOutput, payloads)
}

func resolveDescribePayload(name string, dynamicProjects []bridge.ProjectInfo) (toolDescribePayload, error) {
	if definition, ok := tools.FindBaseDefinition(name); ok {
		return buildToolDescribePayload(*definition), nil
	}

	definition, err := resolveDynamicToolDefinition(name, toolDescribeProject, dynamicProjects)
	if err != nil {
		return toolDescribePayload{}, err
	}
	return buildToolDescribePayload(*definition), nil
}

func resolveDynamicToolDefinition(name string, explicitProject string, projects []bridge.ProjectInfo) (*bridge.ToolDefinition, error) {
	targetProjects := projects
	if explicitProject != "" {
		project, err := bridge.ResolveProject(explicitProject, bridge.CwdHints{}, projects)
		if err != nil {
			return nil, err
		}
		targetProjects = []bridge.ProjectInfo{*project}
	}

	for _, project := range targetProjects {
		for _, tool := range project.Tools {
			if tool.Name == name {
				copyTool := tool
				return &copyTool, nil
			}
		}
	}

	return nil, fmt.Errorf("tool not found: %s", name)
}

func buildToolDescribePayload(definition bridge.ToolDefinition) toolDescribePayload {
	payload := toolDescribePayload{
		Name:           definition.Name,
		Description:    definition.Description,
		Annotations:    definition.Annotations,
		ExamplePayload: tools.BuildExamplePayload(definition.InputSchema),
		InputSchema:    tools.NormalizeSchema(definition.InputSchema),
	}
	if len(definition.OutputSchema) > 0 {
		payload.OutputSchema = tools.NormalizeSchema(definition.OutputSchema)
	}
	return payload
}
