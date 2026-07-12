package tools

import (
	"errors"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
)

var ErrToolNotFound = errors.New("tool not found")

type DaemonCaller interface {
	ListProjects(includeTools bool) (*bridge.ClientListProjectsResult, error)
	ToolCall(tool string, args map[string]any, projectID string, timeout time.Duration) (*bridge.ClientToolCallResult, error)
}

type ExecutionDeps struct {
	Client  DaemonCaller
	Timeout time.Duration
}

type MergedDefinition struct {
	Tool         bridge.ToolDefinition `json:"tool"`
	HasConflicts bool                  `json:"hasConflicts,omitempty"`
	ProjectIDs   []string              `json:"projectIds,omitempty"`
	ProjectNames []string              `json:"projectNames,omitempty"`
}
