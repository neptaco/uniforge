package cmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/unity"
)

func resolveEditorLogTargetWithoutManaged(
	projects []bridge.ProjectInfo,
	projectArg, cwd, fallback string,
) (path string, note string, err error) {
	return resolveEditorLogTarget(
		projects,
		projectArg,
		cwd,
		fallback,
		func(string) (string, bool) { return "", false },
	)
}

func TestResolveEditorLogTargetUsesConnectedProjectLog(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "game")
	consoleLogPath := filepath.Join(t.TempDir(), "Game.log")
	fallback := filepath.Join(t.TempDir(), "Editor.log")
	projects := []bridge.ProjectInfo{
		{
			ID:             projectPath,
			Name:           "Game",
			GitRoot:        t.TempDir(),
			Connected:      true,
			ConsoleLogPath: consoleLogPath,
		},
	}

	path, note, err := resolveEditorLogTarget(
		projects,
		"",
		t.TempDir(),
		fallback,
		func(string) (string, bool) {
			t.Fatal("managed log lookup should not run when consoleLogPath is available")
			return "", false
		},
	)

	if err != nil {
		t.Fatalf("resolveEditorLogTarget failed: %v", err)
	}
	if path != consoleLogPath {
		t.Fatalf("path = %q, want %q", path, consoleLogPath)
	}
	wantNote := "Reading log for connected project Game: " + consoleLogPath
	if note != wantNote {
		t.Fatalf("note = %q, want %q", note, wantNote)
	}
}

func TestResolveEditorLogTargetMatchesExplicitConnectedProject(t *testing.T) {
	root := t.TempDir()
	fallback := filepath.Join(t.TempDir(), "Editor.log")
	betaLogPath := filepath.Join(t.TempDir(), "Beta.log")
	projects := []bridge.ProjectInfo{
		{ID: filepath.Join(root, "alpha"), Name: "Alpha Client", Connected: true, ConsoleLogPath: filepath.Join(t.TempDir(), "Alpha.log")},
		{ID: filepath.Join(root, "beta"), Name: "Beta Client", Connected: true, ConsoleLogPath: betaLogPath},
	}

	for _, explicit := range []string{projects[1].ID, "beta client", "Beta"} {
		t.Run(explicit, func(t *testing.T) {
			path, note, err := resolveEditorLogTargetWithoutManaged(projects, explicit, t.TempDir(), fallback)
			if err != nil {
				t.Fatalf("resolveEditorLogTarget failed: %v", err)
			}
			if path != betaLogPath {
				t.Fatalf("path = %q, want %q", path, betaLogPath)
			}
			wantNote := "Reading log for connected project Beta Client: " + betaLogPath
			if note != wantNote {
				t.Fatalf("note = %q, want %q", note, wantNote)
			}
		})
	}
}

func TestResolveEditorLogTargetMatchesCwdGitRoot(t *testing.T) {
	repositoryRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(repositoryRoot, ".git"), 0o755); err != nil {
		t.Fatalf("create .git: %v", err)
	}
	cwd := filepath.Join(repositoryRoot, "tools", "nested")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("create cwd: %v", err)
	}
	consoleLogPath := filepath.Join(t.TempDir(), "Beta.log")
	projects := []bridge.ProjectInfo{
		{ID: filepath.Join(repositoryRoot, "project"), Name: "Beta", GitRoot: repositoryRoot, Connected: true, ConsoleLogPath: consoleLogPath},
		{ID: filepath.Join(t.TempDir(), "other"), Name: "Other", GitRoot: t.TempDir(), Connected: true, ConsoleLogPath: filepath.Join(t.TempDir(), "Other.log")},
	}

	path, note, err := resolveEditorLogTargetWithoutManaged(
		projects,
		"",
		cwd,
		filepath.Join(t.TempDir(), "Editor.log"),
	)

	if err != nil {
		t.Fatalf("resolveEditorLogTarget failed: %v", err)
	}
	if path != consoleLogPath {
		t.Fatalf("path = %q, want %q", path, consoleLogPath)
	}
	wantNote := "Reading log for connected project Beta: " + consoleLogPath
	if note != wantNote {
		t.Fatalf("note = %q, want %q", note, wantNote)
	}
}

func TestResolveEditorLogTargetReturnsErrorWhenMultipleProjectsCannotBeIdentified(t *testing.T) {
	projects := []bridge.ProjectInfo{
		{ID: filepath.Join(t.TempDir(), "alpha"), Name: "Alpha", Connected: true},
		{ID: filepath.Join(t.TempDir(), "beta"), Name: "Beta", Connected: true},
	}

	path, note, err := resolveEditorLogTargetWithoutManaged(
		projects,
		"",
		t.TempDir(),
		filepath.Join(t.TempDir(), "Editor.log"),
	)

	if err == nil {
		t.Fatal("expected an ambiguous project error")
	}
	if got, want := err.Error(), "multiple Unity projects are connected; specify a project argument"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if path != "" || note != "" {
		t.Fatalf("path = %q, note = %q; want both empty", path, note)
	}
}

func TestResolveEditorLogTargetFallsBackWithoutConnections(t *testing.T) {
	fallback := filepath.Join(t.TempDir(), "Editor.log")
	path, note, err := resolveEditorLogTargetWithoutManaged(nil, "", t.TempDir(), fallback)

	if err != nil {
		t.Fatalf("resolveEditorLogTarget failed: %v", err)
	}
	if path != fallback {
		t.Fatalf("path = %q, want %q", path, fallback)
	}
	if got, want := note, "Reading global Unity Editor log: "+fallback; got != want {
		t.Fatalf("note = %q, want %q", got, want)
	}
}

func TestResolveEditorLogTargetDoesNotUseManagedLogForConnectedLegacyProject(t *testing.T) {
	fallback := filepath.Join(t.TempDir(), "Editor.log")
	projects := []bridge.ProjectInfo{
		{ID: filepath.Join(t.TempDir(), "game"), Name: "Game", Connected: true},
	}

	path, note, err := resolveEditorLogTarget(
		projects,
		"",
		t.TempDir(),
		fallback,
		func(string) (string, bool) {
			t.Fatal("managed log lookup must not run for a connected legacy project")
			return "", false
		},
	)

	if err != nil {
		t.Fatalf("resolveEditorLogTarget failed: %v", err)
	}
	if path != fallback {
		t.Fatalf("path = %q, want %q", path, fallback)
	}
	if got, want := note, "Reading global Unity Editor log: "+fallback; got != want {
		t.Fatalf("note = %q, want %q", got, want)
	}
}

func TestResolveEditorLogTargetUsesManagedLogForOfflineProject(t *testing.T) {
	projectRoot := createLogsTestUnityProject(t)
	managedPath := filepath.Join(t.TempDir(), "managed.log")
	fallback := filepath.Join(t.TempDir(), "Editor.log")

	path, note, err := resolveEditorLogTarget(
		nil,
		projectRoot,
		t.TempDir(),
		fallback,
		func(projectPath string) (string, bool) {
			if projectPath != projectRoot {
				t.Fatalf("project path = %q, want %q", projectPath, projectRoot)
			}
			return managedPath, true
		},
	)

	if err != nil {
		t.Fatalf("resolveEditorLogTarget failed: %v", err)
	}
	if path != managedPath {
		t.Fatalf("path = %q, want %q", path, managedPath)
	}
	wantNote := "Reading managed log for project " + filepath.Base(projectRoot) + ": " + managedPath
	if note != wantNote {
		t.Fatalf("note = %q, want %q", note, wantNote)
	}
}

func TestResolveEditorLogTargetUsesOfflineProjectFromCwdBeforeConnectionCount(t *testing.T) {
	projectRoot := createLogsTestUnityProject(t)
	cwd := filepath.Join(projectRoot, "Assets", "Scripts")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("create cwd: %v", err)
	}
	managedPath := filepath.Join(t.TempDir(), "managed.log")
	projects := []bridge.ProjectInfo{
		{ID: filepath.Join(t.TempDir(), "alpha"), Name: "Alpha", Connected: true},
		{ID: filepath.Join(t.TempDir(), "beta"), Name: "Beta", Connected: true},
	}

	path, _, err := resolveEditorLogTarget(
		projects,
		"",
		cwd,
		filepath.Join(t.TempDir(), "Editor.log"),
		func(projectPath string) (string, bool) {
			if projectPath != projectRoot {
				t.Fatalf("project path = %q, want %q", projectPath, projectRoot)
			}
			return managedPath, true
		},
	)

	if err != nil {
		t.Fatalf("resolveEditorLogTarget failed: %v", err)
	}
	if path != managedPath {
		t.Fatalf("path = %q, want %q", path, managedPath)
	}
}

func TestResolveEditorLogTargetFallsBackWhenOfflineManagedLogIsMissing(t *testing.T) {
	projectRoot := createLogsTestUnityProject(t)
	fallback := filepath.Join(t.TempDir(), "Editor.log")

	path, note, err := resolveEditorLogTarget(
		nil,
		projectRoot,
		t.TempDir(),
		fallback,
		func(projectPath string) (string, bool) {
			if projectPath != projectRoot {
				t.Fatalf("project path = %q, want %q", projectPath, projectRoot)
			}
			return filepath.Join(t.TempDir(), "managed.log"), false
		},
	)

	if err != nil {
		t.Fatalf("resolveEditorLogTarget failed: %v", err)
	}
	if path != fallback {
		t.Fatalf("path = %q, want %q", path, fallback)
	}
	if got, want := note, "Reading global Unity Editor log: "+fallback; got != want {
		t.Fatalf("note = %q, want %q", got, want)
	}
}

func TestResolveEditorLogTargetRejectsUnknownExplicitProject(t *testing.T) {
	cwdProject := createLogsTestUnityProject(t)
	projects := []bridge.ProjectInfo{{ID: cwdProject, Name: "Connected", Connected: true}}

	path, note, err := resolveEditorLogTargetWithoutManaged(
		projects,
		"missing-project",
		cwdProject,
		filepath.Join(t.TempDir(), "Editor.log"),
	)

	if err == nil {
		t.Fatal("expected project-not-found error")
	}
	if path != "" || note != "" {
		t.Fatalf("path = %q, note = %q; want both empty", path, note)
	}
}

func TestRunLogSuppressesResolutionNoteInRawMode(t *testing.T) {
	testRoot := t.TempDir()
	t.Setenv("HOME", testRoot)
	t.Setenv("LOCALAPPDATA", testRoot)
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(testRoot, "runtime"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(testRoot, "state"))
	t.Chdir(testRoot)

	fallback, err := unity.GetEditorLogPath()
	if err != nil {
		t.Fatalf("get Editor.log path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(fallback), 0o755); err != nil {
		t.Fatalf("create Editor.log directory: %v", err)
	}
	if err := os.WriteFile(fallback, []byte("raw log line\n"), 0o644); err != nil {
		t.Fatalf("write Editor.log: %v", err)
	}

	previousRaw := logRaw
	previousEditor := logEditor
	previousFollow := logFollow
	previousLines := logLines
	t.Cleanup(func() {
		logRaw = previousRaw
		logEditor = previousEditor
		logFollow = previousFollow
		logLines = previousLines
	})
	logRaw = true
	logEditor = false
	logFollow = false
	logLines = 100

	stderr := captureLogsTestStderr(t, func() {
		if err := runLog(logCmd, nil); err != nil {
			t.Fatalf("runLog failed: %v", err)
		}
	})
	if strings.Contains(stderr, "Reading global Unity Editor log") {
		t.Fatalf("raw output included resolution note: %q", stderr)
	}
}

func captureLogsTestStderr(t *testing.T, action func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	previousStderr := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = previousStderr }()

	action()
	if err := writer.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}
	return string(output)
}

func createLogsTestUnityProject(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	for _, directory := range []string{"Assets", "ProjectSettings"} {
		if err := os.MkdirAll(filepath.Join(projectRoot, directory), 0o755); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	return projectRoot
}
