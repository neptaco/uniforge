package cmd

import (
	"strings"
	"testing"

	"github.com/neptaco/uniforge/pkg/bridge"
)

func TestBuildToolListOmitsInputSchema(t *testing.T) {
	project := bridge.ProjectInfo{
		Name: "Sample Project",
		Tools: []bridge.ToolDefinition{{
			Name:        "custom-tool",
			Description: "Custom tool",
			InputSchema: map[string]any{
				"type": "object",
			},
		}},
	}

	result := buildToolList(project)

	if result.Project != "Sample Project" {
		t.Fatalf("project = %q, want %q", result.Project, "Sample Project")
	}

	var custom *toolEntry
	for i := range result.Tools {
		if result.Tools[i].Name == "custom-tool" {
			custom = &result.Tools[i]
			break
		}
	}

	if custom == nil {
		t.Fatalf("custom-tool entry not found")
	}
	if custom.Description != "Custom tool" {
		t.Fatalf("description = %q, want %q", custom.Description, "Custom tool")
	}
}

func TestRenderToolListYAMLOmitsSchemaAndAppendsHint(t *testing.T) {
	result := toolListResult{
		Project: "test",
		Tools: []toolEntry{{
			Name:        "custom-tool",
			Description: "Custom tool",
		}},
	}
	stdout, stderr, err := renderToolList("yaml", result)
	if err != nil {
		t.Fatalf("renderToolList() error = %v", err)
	}

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if strings.Contains(stdout, "inputSchema") {
		t.Fatalf("stdout unexpectedly contains inputSchema: %s", stdout)
	}
	if !strings.Contains(stdout, "# "+toolListDescribeHint) {
		t.Fatalf("stdout does not contain describe hint: %s", stdout)
	}
}

func TestRenderToolListJSONOmitsSchema(t *testing.T) {
	result := toolListResult{
		Project: "test",
		Tools: []toolEntry{{
			Name:        "custom-tool",
			Description: "Custom tool",
		}},
	}
	stdout, stderr, err := renderToolList("json", result)
	if err != nil {
		t.Fatalf("renderToolList() error = %v", err)
	}

	if strings.Contains(stdout, "inputSchema") {
		t.Fatalf("stdout unexpectedly contains inputSchema: %s", stdout)
	}
	if !strings.HasPrefix(stdout, "{\n") {
		t.Fatalf("stdout = %q, want JSON object", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}
