package mcp

import (
	"testing"

	"github.com/neptaco/uniforge/pkg/bridge"
)

func TestMergeDynamicToolDefinitions(t *testing.T) {
	projects := []bridge.ProjectInfo{
		{
			ID:   "/tmp/alpha",
			Name: "Alpha",
			Tools: []bridge.ToolDefinition{
				{
					Name:        "editor-state",
					Description: "Read the current editor state",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		},
		{
			ID:   "/tmp/beta",
			Name: "Beta",
			Tools: []bridge.ToolDefinition{
				{
					Name:        "editor-state",
					Description: "Read the current editor state",
					InputSchema: map[string]any{"type": "object"},
				},
				{
					Name:        "hierarchy",
					Description: "Inspect the hierarchy",
					InputSchema: map[string]any{"type": "object"},
				},
			},
		},
	}

	tools := MergeDynamicToolDefinitions(projects)
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(tools))
	}

	if tools[0].Name != "editor-state" {
		t.Fatalf("tools[0].Name = %q, want %q", tools[0].Name, "editor-state")
	}
	if len(tools[0].Sources) != 2 {
		t.Fatalf("len(tools[0].Sources) = %d, want 2", len(tools[0].Sources))
	}
	if tools[0].HasConflicts {
		t.Fatalf("tools[0].HasConflicts = true, want false")
	}
}

func TestMergeDynamicToolDefinitionsMarksConflict(t *testing.T) {
	projects := []bridge.ProjectInfo{
		{
			ID:   "/tmp/alpha",
			Name: "Alpha",
			Tools: []bridge.ToolDefinition{{
				Name:        "editor-state",
				Description: "Read state",
				InputSchema: map[string]any{"type": "object"},
			}},
		},
		{
			ID:   "/tmp/beta",
			Name: "Beta",
			Tools: []bridge.ToolDefinition{{
				Name:        "editor-state",
				Description: "Read editor state with extra detail",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{"verbose": map[string]any{"type": "boolean"}}},
			}},
		},
	}

	tools := MergeDynamicToolDefinitions(projects)
	if len(tools) != 1 {
		t.Fatalf("len(tools) = %d, want 1", len(tools))
	}
	if !tools[0].HasConflicts {
		t.Fatalf("tools[0].HasConflicts = false, want true")
	}
}
