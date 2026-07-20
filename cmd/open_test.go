package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neptaco/uniforge/pkg/unity"
)

func TestOpenCommandHasNoLogFileFlag(t *testing.T) {
	flag := openCmd.Flags().Lookup("no-log-file")
	if flag == nil {
		t.Fatal("open command does not define --no-log-file")
	}
	if flag.DefValue != "false" {
		t.Fatalf("--no-log-file default = %q, want false", flag.DefValue)
	}
}

func TestRunOpenPropagatesNoLogFileOption(t *testing.T) {
	projectPath := createOpenTestUnityProject(t)
	previousNoLogFile := openNoLogFile
	previousVersion := openVersion
	previousLauncher := launchEditorWithOptions
	t.Cleanup(func() {
		openNoLogFile = previousNoLogFile
		openVersion = previousVersion
		launchEditorWithOptions = previousLauncher
	})
	openVersion = ""

	for _, test := range []struct {
		name      string
		noLogFile bool
	}{
		{name: "managed log enabled by default", noLogFile: false},
		{name: "managed log disabled", noLogFile: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			openNoLogFile = test.noLogFile
			called := false
			var gotProjectPath string
			var gotVersion string
			var gotOptions unity.EditorOpenOptions
			launchEditorWithOptions = func(actualProjectPath, actualVersion string, options unity.EditorOpenOptions) error {
				called = true
				gotProjectPath = actualProjectPath
				gotVersion = actualVersion
				gotOptions = options
				return nil
			}

			if err := runOpen(openCmd, []string{projectPath}); err != nil {
				t.Fatalf("runOpen failed: %v", err)
			}
			if !called {
				t.Fatal("editor launcher was not called")
			}
			if gotProjectPath != projectPath {
				t.Fatalf("project path = %q, want %q", gotProjectPath, projectPath)
			}
			if gotVersion != "6000.0.54f1" {
				t.Fatalf("version = %q, want 6000.0.54f1", gotVersion)
			}
			if gotOptions.NoLogFile != test.noLogFile {
				t.Fatalf("NoLogFile = %t, want %t", gotOptions.NoLogFile, test.noLogFile)
			}
		})
	}
}

func createOpenTestUnityProject(t *testing.T) string {
	t.Helper()
	projectPath := t.TempDir()
	projectSettingsPath := filepath.Join(projectPath, "ProjectSettings")
	if err := os.MkdirAll(projectSettingsPath, 0o755); err != nil {
		t.Fatalf("create ProjectSettings: %v", err)
	}
	versionFile := filepath.Join(projectSettingsPath, "ProjectVersion.txt")
	if err := os.WriteFile(versionFile, []byte("m_EditorVersion: 6000.0.54f1\n"), 0o644); err != nil {
		t.Fatalf("write ProjectVersion.txt: %v", err)
	}
	return projectPath
}
