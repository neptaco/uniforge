package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/updater"
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
	noteUnityPackageUpdates(projects, updater.UnityPackageLatest{Version: "0.12.0"}, func(format string, args ...any) {
		notes = append(notes, fmt.Sprintf(format, args...))
	})

	want := []string{
		"Unity package update available for OldGame: 0.11.0 -> 0.12.0 (see uniforge-unity tags)",
	}
	if !reflect.DeepEqual(notes, want) {
		t.Fatalf("notes = %#v, want %#v", notes, want)
	}
}

func TestNoteUnityPackageUpdatesUsesProjectUnityCompatibility(t *testing.T) {
	createProject := func(t *testing.T, version string) string {
		t.Helper()
		projectPath := t.TempDir()
		if version == "" {
			return projectPath
		}
		settingsPath := filepath.Join(projectPath, "ProjectSettings")
		if err := os.MkdirAll(settingsPath, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(
			filepath.Join(settingsPath, "ProjectVersion.txt"),
			[]byte("m_EditorVersion: "+version+"\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		return projectPath
	}

	tests := []struct {
		name    string
		project string
		latest  updater.UnityPackageLatest
		want    string
	}{
		{
			name:    "compatible",
			project: createProject(t, "6000.2.0f1"),
			latest:  updater.UnityPackageLatest{Version: "0.13.0", Unity: "6000.2", UnityRelease: "0f1"},
			want:    "Unity package update available for Game: 0.12.0 -> 0.13.0 (see uniforge-unity tags)",
		},
		{
			name:    "incompatible",
			project: createProject(t, "6000.0.70f1"),
			latest:  updater.UnityPackageLatest{Version: "0.13.0", Unity: "6000.2", UnityRelease: "0f1"},
			want:    "Unity package 0.13.0 available but requires Unity >= 6000.2 (project has 6000.0.70f1)",
		},
		{
			name:    "unknown requirement",
			project: createProject(t, "6000.0.70f1"),
			latest:  updater.UnityPackageLatest{Version: "0.13.0"},
			want:    "Unity package update available for Game: 0.12.0 -> 0.13.0 (see uniforge-unity tags)",
		},
		{
			name:    "project version unreadable",
			project: createProject(t, ""),
			latest:  updater.UnityPackageLatest{Version: "0.13.0", Unity: "6000.2", UnityRelease: "0f1"},
			want:    "Unity package update available for Game: 0.12.0 -> 0.13.0 (see uniforge-unity tags)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var notes []string
			noteUnityPackageUpdates([]bridge.ProjectInfo{{
				ID:             test.project,
				Name:           "Game",
				PackageVersion: "0.12.0",
			}}, test.latest, func(format string, args ...any) {
				notes = append(notes, fmt.Sprintf(format, args...))
			})
			if want := []string{test.want}; !reflect.DeepEqual(notes, want) {
				t.Fatalf("notes = %#v, want %#v", notes, want)
			}
		})
	}
}

func TestNoteUnityPackageUpdatesSkipsUnknownLatestVersion(t *testing.T) {
	projects := []bridge.ProjectInfo{{Name: "Game", PackageVersion: "0.11.0"}}

	for _, latestVersion := range []string{"", "not-semver"} {
		t.Run(latestVersion, func(t *testing.T) {
			var notes []string
			noteUnityPackageUpdates(projects, updater.UnityPackageLatest{Version: latestVersion}, func(format string, args ...any) {
				notes = append(notes, fmt.Sprintf(format, args...))
			})
			if len(notes) != 0 {
				t.Fatalf("notes = %#v, want none", notes)
			}
		})
	}
}
