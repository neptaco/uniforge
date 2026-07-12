package tools

import (
	"testing"

	"github.com/neptaco/uniforge/pkg/bridge"
)

func TestMergeDynamicDefinitionsAddsProjectSelector(t *testing.T) {
	projects := []bridge.ProjectInfo{
		{
			ID:   "/tmp/a",
			Name: "Alpha",
			Tools: []bridge.ToolDefinition{{
				Name:        "editor-state",
				Description: "Read editor state",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"verbose": map[string]any{"type": "boolean"},
					},
				},
			}},
		},
		{
			ID:   "/tmp/b",
			Name: "Beta",
			Tools: []bridge.ToolDefinition{{
				Name:        "editor-state",
				Description: "Read editor state",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"include_prefabs": map[string]any{"type": "boolean"},
					},
				},
			}},
		},
	}

	merged := MergeDynamicDefinitions(projects, map[string]struct{}{}, true)
	if len(merged) != 1 {
		t.Fatalf("len(merged) = %d, want 1", len(merged))
	}

	properties, ok := merged[0].Tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing from merged schema")
	}
	if _, ok := properties["project_id"]; !ok {
		t.Fatalf("project_id property missing from merged schema")
	}
	if _, ok := properties["verbose"]; !ok {
		t.Fatalf("verbose property missing from merged schema")
	}
	if _, ok := properties["include_prefabs"]; !ok {
		t.Fatalf("include_prefabs property missing from merged schema")
	}
}
