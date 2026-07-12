package cmd

import (
	"fmt"

	"github.com/neptaco/uniforge/pkg/tools"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var (
	toolCallJSON            string
	toolCallDryRun          bool
	toolCallProject         string
	toolCallOutput          string
	toolCallTimeoutMS       int
	toolCallAutoStartDaemon bool
	toolCallVerbose         bool
)

var toolCallCmd = &cobra.Command{
	Use:   "call <tool>",
	Short: "Call a Unity tool through the Go daemon",
	Long: `Call a tool exposed by a connected Unity project.

Examples:
  # Call a tool on the current project
  uniforge tool call compile-status

  # Pass arguments as a positional JSON arg
  uniforge tool call logs '{"limit":50}'

  # Or via --json flag
  uniforge tool call logs --json '{"limit":50}'

  # Target a specific connected project
  uniforge tool call hierarchy --project my-project -o json`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runToolCall,
}

func init() {
	toolCmd.AddCommand(toolCallCmd)

	toolCallCmd.Flags().StringVar(&toolCallJSON, "json", "", "Tool arguments as a JSON object")
	toolCallCmd.Flags().BoolVar(&toolCallDryRun, "dry-run", false, "Show what would be called without running")
	toolCallCmd.Flags().StringVarP(&toolCallProject, "project", "p", "", "Project ID or name")
	toolCallCmd.Flags().StringVarP(&toolCallOutput, "output", "o", "yaml", "Output format: yaml, json")
	toolCallCmd.Flags().IntVar(&toolCallTimeoutMS, "timeout", 300000, "Request timeout in milliseconds")
	toolCallCmd.Flags().BoolVar(&toolCallAutoStartDaemon, "auto-start-daemon", true, "Start the daemon automatically if needed")
	toolCallCmd.Flags().BoolVarP(&toolCallVerbose, "verbose", "v", false, "Show daemon and connection logs")
}

func runToolCall(cmd *cobra.Command, args []string) error {
	toolName := args[0]
	// 第2引数があれば --json より優先
	jsonInput := toolCallJSON
	if len(args) > 1 {
		jsonInput = args[1]
	}
	toolArgs, err := readToolArgs(jsonInput)
	if err != nil {
		return fmt.Errorf("failed to parse tool arguments: %w", err)
	}

	baseTool, isBaseTool := tools.FindBaseDefinition(toolName)

	if toolCallDryRun {
		projectLabel := "n/a"
		if !isBaseTool || toolName == "list-projects" {
			if toolCallProject != "" {
				projectLabel = toolCallProject
			} else {
				projectLabel = "auto"
			}
		}
		return writeStructuredOutput(toolCallOutput, map[string]any{
			"dryRun":  true,
			"tool":    toolName,
			"args":    toolArgs,
			"project": projectLabel,
		})
	}

	client := newToolClient(toolClientOptions{
		project:         toolCallProject,
		output:          toolCallOutput,
		timeoutMS:       toolCallTimeoutMS,
		autoStartDaemon: toolCallAutoStartDaemon,
	})
	defer func() { _ = client.Close() }()

	needsDaemon := !isBaseTool || toolName == "list-projects"
	if needsDaemon {
		writeToolVerbose(toolCallVerbose, "Connecting to daemon")
		if err := client.Connect(); err != nil {
			return err
		}

		writeToolVerbose(toolCallVerbose, "Registering tool client")
		registerResult, err := client.Register()
		if err != nil {
			return err
		}
		if registerResult.Warning != "" {
			ui.Warn("%s", registerResult.Warning)
		}
	}

	if isBaseTool {
		writeToolVerbose(toolCallVerbose, "Calling built-in tool %q", baseTool.Name)
		payload, err := tools.Execute(tools.ExecutionDeps{
			Client:  client,
			Timeout: durationFromMillis(toolCallTimeoutMS),
		}, toolName, toolArgs)
		if err != nil {
			return err
		}
		if payload == nil {
			ui.Success("Tool %s completed", toolName)
			return nil
		}
		return writeStructuredOutput(toolCallOutput, payload)
	}

	project, _, err := resolveToolProject(client, toolCallProject, false)
	if err != nil {
		return err
	}
	writeToolVerbose(toolCallVerbose, "Resolved Unity project %q", project.ID)

	result, err := client.ToolCall(toolName, toolArgs, project.ID, durationFromMillis(toolCallTimeoutMS))
	if err != nil {
		return err
	}
	if !result.Success {
		errMsg := result.Error
		if errMsg == "" {
			errMsg = "unknown error"
		}
		return fmt.Errorf("tool %s failed: %s\nhint: run `uniforge tool describe %s` to check the expected schema", toolName, errMsg, toolName)
	}

	if result.Result == nil {
		ui.Success("Tool %s completed", toolName)
		return nil
	}

	return writeStructuredOutput(toolCallOutput, result.Result)
}
