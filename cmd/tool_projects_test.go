package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neptaco/uniforge/pkg/bridge"
	"gopkg.in/yaml.v3"
)

func TestBuildToolProjectEntriesIncludesConsoleLogPath(t *testing.T) {
	consoleLogPath := filepath.Join(t.TempDir(), "Editor.log")
	projects := []bridge.ProjectInfo{
		{
			ID:             filepath.Join(t.TempDir(), "project"),
			Name:           "Game",
			GitRoot:        t.TempDir(),
			ConsoleLogPath: consoleLogPath,
			Connected:      true,
			Tools:          []bridge.ToolDefinition{{Name: "refresh"}},
		},
	}

	entries := buildToolProjectEntries(projects)
	if len(entries) != 1 {
		t.Fatalf("entry count = %d, want 1", len(entries))
	}
	if entries[0].ConsoleLogPath != consoleLogPath {
		t.Fatalf("consoleLogPath = %q, want %q", entries[0].ConsoleLogPath, consoleLogPath)
	}
	if len(entries[0].Tools) != 1 || entries[0].Tools[0] != "refresh" {
		t.Fatalf("tools = %#v, want refresh", entries[0].Tools)
	}
}

func TestToolProjectEntryConsoleLogPathSerialization(t *testing.T) {
	tests := []struct {
		name        string
		entry       toolProjectEntry
		wantPresent bool
	}{
		{name: "non-empty", entry: toolProjectEntry{ConsoleLogPath: "Editor.log"}, wantPresent: true},
		{name: "empty", entry: toolProjectEntry{}, wantPresent: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			jsonData, err := json.Marshal(test.entry)
			if err != nil {
				t.Fatalf("marshal JSON: %v", err)
			}
			yamlData, err := yaml.Marshal(test.entry)
			if err != nil {
				t.Fatalf("marshal YAML: %v", err)
			}

			if got := strings.Contains(string(jsonData), "consoleLogPath"); got != test.wantPresent {
				t.Fatalf("JSON consoleLogPath presence = %t, want %t: %s", got, test.wantPresent, jsonData)
			}
			if got := strings.Contains(string(yamlData), "consoleLogPath"); got != test.wantPresent {
				t.Fatalf("YAML consoleLogPath presence = %t, want %t: %s", got, test.wantPresent, yamlData)
			}
		})
	}
}
