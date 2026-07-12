package mcp

import (
	"context"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
)

// ToolSource identifies the Unity project that provides a tool definition.
type ToolSource struct {
	ID   string `json:"id" yaml:"id"`
	Name string `json:"name" yaml:"name"`
}

// ToolMetadata describes a tool exposed through the MCP surface.
type ToolMetadata struct {
	bridge.ToolDefinition `json:",inline" yaml:",inline"`
	Sources               []ToolSource `json:"sources,omitempty" yaml:"sources,omitempty"`
	HasConflicts          bool         `json:"hasConflicts,omitempty" yaml:"hasConflicts,omitempty"`
}

// ListToolsOptions controls metadata lookup.
type ListToolsOptions struct {
	Project string
}

// DescribeToolOptions controls describe lookups.
type DescribeToolOptions struct {
	Project string
}

// ExecuteToolOptions controls tool execution.
type ExecuteToolOptions struct {
	Project string
	Timeout time.Duration
}

// ExecuteToolResult is the normalized result of a tool invocation.
type ExecuteToolResult struct {
	Success bool   `json:"success" yaml:"success"`
	Result  any    `json:"result,omitempty" yaml:"result,omitempty"`
	Error   string `json:"error,omitempty" yaml:"error,omitempty"`
}

// ToolMetadataProvider supplies tool metadata independently from its execution backend.
type ToolMetadataProvider interface {
	ListToolMetadata(context.Context, ListToolsOptions) ([]ToolMetadata, error)
	DescribeTool(context.Context, string, DescribeToolOptions) (*ToolMetadata, error)
}

// ToolExecutor executes a named tool independently from its metadata source.
type ToolExecutor interface {
	ExecuteTool(context.Context, string, map[string]any, ExecuteToolOptions) (*ExecuteToolResult, error)
}

// Runtime is the minimal surface needed by the MCP commands and stdio server.
type Runtime interface {
	ToolMetadataProvider
	ToolExecutor
}
