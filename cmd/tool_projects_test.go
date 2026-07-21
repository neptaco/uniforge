package cmd

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
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
			PackageVersion: "0.11.0",
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
	if entries[0].PackageVersion != "0.11.0" {
		t.Fatalf("packageVersion = %q, want %q", entries[0].PackageVersion, "0.11.0")
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

func TestToolProjectEntryPackageVersionSerialization(t *testing.T) {
	tests := []struct {
		name        string
		entry       toolProjectEntry
		wantPresent bool
	}{
		{name: "non-empty", entry: toolProjectEntry{PackageVersion: "0.11.0"}, wantPresent: true},
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

			if got := strings.Contains(string(jsonData), "packageVersion"); got != test.wantPresent {
				t.Fatalf("JSON packageVersion presence = %t, want %t: %s", got, test.wantPresent, jsonData)
			}
			if got := strings.Contains(string(yamlData), "packageVersion"); got != test.wantPresent {
				t.Fatalf("YAML packageVersion presence = %t, want %t: %s", got, test.wantPresent, yamlData)
			}
		})
	}
}

func TestNoteUnityPackageUpdatesReportsOnlyOlderPackages(t *testing.T) {
	projects := []bridge.ProjectInfo{
		{Name: "OldGame", PackageVersion: "0.11.0"},
		{Name: "CurrentGame", PackageVersion: "0.12.0"},
		{Name: "NewerGame", PackageVersion: "0.13.0"},
		{Name: "LegacyGame"},
		{Name: "InvalidGame", PackageVersion: "not-semver"},
	}

	var notes []string
	noteUnityPackageUpdates(projects, "0.12.0", func(format string, args ...any) {
		notes = append(notes, fmt.Sprintf(format, args...))
	})

	want := []string{
		"Unity package update available for OldGame: 0.11.0 -> 0.12.0 (see uniforge-unity tags)",
	}
	if !reflect.DeepEqual(notes, want) {
		t.Fatalf("notes = %#v, want %#v", notes, want)
	}
}

func TestNoteUnityPackageUpdatesSkipsUnknownLatestVersion(t *testing.T) {
	projects := []bridge.ProjectInfo{{Name: "Game", PackageVersion: "0.11.0"}}

	for _, latestVersion := range []string{"", "not-semver"} {
		t.Run(latestVersion, func(t *testing.T) {
			var notes []string
			noteUnityPackageUpdates(projects, latestVersion, func(format string, args ...any) {
				notes = append(notes, fmt.Sprintf(format, args...))
			})
			if len(notes) != 0 {
				t.Fatalf("notes = %#v, want none", notes)
			}
		})
	}
}
