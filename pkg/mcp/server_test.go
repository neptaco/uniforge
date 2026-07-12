package mcp

import (
	"context"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/neptaco/uniforge/pkg/bridge"
)

type stubRuntime struct {
	tools  []ToolMetadata
	result *ExecuteToolResult
}

func (s *stubRuntime) ListToolMetadata(context.Context, ListToolsOptions) ([]ToolMetadata, error) {
	return s.tools, nil
}

func (s *stubRuntime) DescribeTool(ctx context.Context, name string, options DescribeToolOptions) (*ToolMetadata, error) {
	tools, err := s.ListToolMetadata(ctx, ListToolsOptions(options))
	if err != nil {
		return nil, err
	}
	return FindToolMetadata(tools, name)
}

func (s *stubRuntime) ExecuteTool(context.Context, string, map[string]any, ExecuteToolOptions) (*ExecuteToolResult, error) {
	return s.result, nil
}

func TestServerRunListsAndExecutesTools(t *testing.T) {
	runtime := &stubRuntime{
		tools: []ToolMetadata{{
			ToolDefinition: bridgeToolDefinition(),
			Sources:        []ToolSource{{ID: "/tmp/project", Name: "Project"}},
		}},
		result: &ExecuteToolResult{
			Success: true,
			Result: map[string]any{
				"status": "ok",
			},
		},
	}

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	server := NewServer(runtime, ServerOptions{
		Name:            "uniforge",
		Version:         "test",
		RefreshInterval: time.Hour,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Run(ctx, serverTransport)
	}()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer func() { _ = session.Close() }()

	listResult, err := session.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	if len(listResult.Tools) != 1 {
		t.Fatalf("len(listResult.Tools) = %d, want 1", len(listResult.Tools))
	}
	if got := listResult.Tools[0].Name; got != "editor-state" {
		t.Fatalf("tool name = %q, want %q", got, "editor-state")
	}

	callResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "editor-state",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if callResult.IsError {
		t.Fatalf("CallTool().IsError = true, want false")
	}
	if callResult.StructuredContent == nil {
		t.Fatalf("CallTool().StructuredContent = nil, want object")
	}
}

func bridgeToolDefinition() bridgeTool {
	return bridge.ToolDefinition{
		Name:        "editor-state",
		Description: "Read the current editor state",
		InputSchema: map[string]any{"type": "object"},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{"type": "string"},
			},
		},
		Annotations: map[string]any{"readOnlyHint": true},
	}
}

type bridgeTool = bridge.ToolDefinition
