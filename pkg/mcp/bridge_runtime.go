package mcp

import (
	"context"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/daemon"
	toolpkg "github.com/neptaco/uniforge/pkg/tools"
)

const defaultRequestTimeout = 5 * time.Minute

// BridgeRuntimeOptions configures the bridge-backed MCP runtime.
type BridgeRuntimeOptions struct {
	DaemonConfig    daemon.Config
	AutoStartDaemon bool
	RequestTimeout  time.Duration
	CwdHints        bridge.CwdHints
}

// BridgeRuntime adapts the existing bridge daemon to the MCP interfaces.
type BridgeRuntime struct {
	daemonConfig    daemon.Config
	autoStartDaemon bool
	requestTimeout  time.Duration
	cwdHints        bridge.CwdHints
}

// NewBridgeRuntime creates a runtime backed by the Go bridge daemon.
func NewBridgeRuntime(options BridgeRuntimeOptions) *BridgeRuntime {
	timeout := options.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}

	hints := options.CwdHints
	if hints == (bridge.CwdHints{}) {
		hints = bridge.ResolveFromCwd("")
	}

	return &BridgeRuntime{
		daemonConfig:    options.DaemonConfig,
		autoStartDaemon: options.AutoStartDaemon,
		requestTimeout:  timeout,
		cwdHints:        hints,
	}
}

// ListToolMetadata returns merged tool metadata from connected Unity projects.
func (r *BridgeRuntime) ListToolMetadata(_ context.Context, options ListToolsOptions) ([]ToolMetadata, error) {
	projects, err := r.listProjects(true, r.requestTimeout)
	if err != nil {
		return nil, err
	}

	scoped, err := r.scopeProjects(projects, options.Project)
	if err != nil {
		return nil, err
	}

	metadata := StaticToolMetadata()
	metadata = append(metadata, MergeDynamicToolDefinitions(scoped)...)
	return metadata, nil
}

// DescribeTool returns merged metadata for a single tool.
func (r *BridgeRuntime) DescribeTool(ctx context.Context, name string, options DescribeToolOptions) (*ToolMetadata, error) {
	tools, err := r.ListToolMetadata(ctx, ListToolsOptions(options))
	if err != nil {
		return nil, err
	}

	return FindToolMetadata(tools, name)
}

// ExecuteTool executes a tool through the bridge daemon.
func (r *BridgeRuntime) ExecuteTool(ctx context.Context, name string, arguments map[string]any, options ExecuteToolOptions) (*ExecuteToolResult, error) {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = r.requestTimeout
	}

	if toolpkg.IsBaseTool(name) {
		var client *bridge.Client
		if requiresDaemonTool(name) {
			client = bridge.NewClient(bridge.ClientOptions{
				AutoStartDaemon: r.autoStartDaemon,
				RequestTimeout:  timeout,
			})
			defer func() { _ = client.Close() }()

			if err := client.Connect(); err != nil {
				return nil, err
			}
			if _, err := client.Register(); err != nil {
				return nil, err
			}
		}

		payload, err := toolpkg.Execute(toolpkg.ExecutionDeps{
			Client:  client,
			Timeout: timeout,
		}, name, arguments)
		if err != nil {
			return nil, err
		}

		return &ExecuteToolResult{
			Success: true,
			Result:  payload,
		}, nil
	}

	client := bridge.NewClient(bridge.ClientOptions{
		DaemonConfig:    r.daemonConfig,
		AutoStartDaemon: r.autoStartDaemon,
		RequestTimeout:  timeout,
	})
	defer func() { _ = client.Close() }()

	if err := client.Connect(); err != nil {
		return nil, err
	}
	if _, err := client.Register(); err != nil {
		return nil, err
	}

	project, err := r.resolveProject(client, options.Project)
	if err != nil {
		return nil, err
	}

	result, err := client.ToolCall(name, arguments, project.ID, timeout)
	if err != nil {
		return nil, err
	}

	return &ExecuteToolResult{
		Success: result.Success,
		Result:  result.Result,
		Error:   result.Error,
	}, nil
}

func (r *BridgeRuntime) listProjects(includeTools bool, timeout time.Duration) ([]bridge.ProjectInfo, error) {
	client := bridge.NewClient(bridge.ClientOptions{
		DaemonConfig:    r.daemonConfig,
		AutoStartDaemon: r.autoStartDaemon,
		RequestTimeout:  timeout,
	})
	defer func() { _ = client.Close() }()

	if err := client.Connect(); err != nil {
		return nil, err
	}
	if _, err := client.Register(); err != nil {
		return nil, err
	}

	result, err := client.ListProjects(includeTools)
	if err != nil {
		return nil, err
	}

	return result.Projects, nil
}

func (r *BridgeRuntime) scopeProjects(projects []bridge.ProjectInfo, explicitProject string) ([]bridge.ProjectInfo, error) {
	if explicitProject == "" {
		return projects, nil
	}

	project, err := bridge.ResolveProject(explicitProject, r.cwdHints, projects)
	if err != nil {
		return nil, err
	}

	return []bridge.ProjectInfo{*project}, nil
}

func (r *BridgeRuntime) resolveProject(client *bridge.Client, explicitProject string) (*bridge.ProjectInfo, error) {
	projectsResult, err := client.ListProjects(false)
	if err != nil {
		return nil, err
	}

	return bridge.ResolveProject(explicitProject, r.cwdHints, projectsResult.Projects)
}

func requiresDaemonTool(name string) bool {
	switch name {
	case "list-projects":
		return true
	default:
		return false
	}
}
