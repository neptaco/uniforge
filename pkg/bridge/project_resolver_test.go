package bridge

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMatchProjectPrefersProjectPath(t *testing.T) {
	repositoryRoot := t.TempDir()
	alphaPath := filepath.Join(repositoryRoot, "alpha")
	betaPath := filepath.Join(repositoryRoot, "beta")
	projects := []ProjectInfo{
		{ID: alphaPath, Name: "Alpha", GitRoot: repositoryRoot},
		{ID: betaPath, Name: "Beta", GitRoot: repositoryRoot},
	}

	match := MatchProject(CwdHints{
		ProjectPath: betaPath,
		GitRoot:     repositoryRoot,
	}, projects)

	if match == nil || match.ID != betaPath {
		t.Fatalf("expected %s, got %#v", betaPath, match)
	}
}

func TestMatchProjectRequiresUniqueGitRoot(t *testing.T) {
	repositoryRoot := t.TempDir()
	projects := []ProjectInfo{
		{ID: filepath.Join(repositoryRoot, "alpha"), Name: "Alpha", GitRoot: repositoryRoot},
		{ID: filepath.Join(repositoryRoot, "beta"), Name: "Beta", GitRoot: repositoryRoot},
	}

	match := MatchProject(CwdHints{GitRoot: repositoryRoot}, projects)
	if match != nil {
		t.Fatalf("expected nil for ambiguous git root, got %#v", match)
	}
}

func TestResolveProjectMatchesExplicitName(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "alpha")
	projects := []ProjectInfo{{ID: projectPath, Name: "Alpha"}}

	project, err := ResolveProject("alpha", CwdHints{}, projects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project == nil || project.ID != projectPath {
		t.Fatalf("unexpected project: %#v", project)
	}
}

func TestResolveProjectOrPathMatchesExplicitConnectedProject(t *testing.T) {
	root := t.TempDir()
	projects := []ProjectInfo{
		{ID: filepath.Join(root, "alpha"), Name: "Alpha Client"},
		{ID: filepath.Join(root, "beta"), Name: "Beta Client"},
	}

	tests := []struct {
		name     string
		explicit string
		wantID   string
	}{
		{name: "id", explicit: projects[1].ID, wantID: projects[1].ID},
		{name: "exact name", explicit: "alpha client", wantID: projects[0].ID},
		{name: "partial name", explicit: "Beta", wantID: projects[1].ID},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project, offlinePath, err := ResolveProjectOrPath(test.explicit, t.TempDir(), projects)
			if err != nil {
				t.Fatalf("ResolveProjectOrPath failed: %v", err)
			}
			if project == nil || project.ID != test.wantID {
				t.Fatalf("project = %#v, want ID %q", project, test.wantID)
			}
			if offlinePath != "" {
				t.Fatalf("offline path = %q, want empty", offlinePath)
			}
		})
	}
}

func TestResolveProjectOrPathResolvesRelativeExplicitIDFromProvidedCwd(t *testing.T) {
	processCwd := t.TempDir()
	apiCwd := t.TempDir()
	t.Chdir(processCwd)
	projects := []ProjectInfo{
		{ID: filepath.Join(processCwd, "game"), Name: "Process Project"},
		{ID: filepath.Join(apiCwd, "game"), Name: "API Project"},
	}

	project, offlinePath, err := ResolveProjectOrPath("game", apiCwd, projects)
	if err != nil {
		t.Fatalf("ResolveProjectOrPath failed: %v", err)
	}
	if project == nil || project.ID != projects[1].ID {
		t.Fatalf("project = %#v, want %#v", project, projects[1])
	}
	if offlinePath != "" {
		t.Fatalf("offline path = %q, want empty", offlinePath)
	}
}

func TestResolveProjectOrPathRejectsAmbiguousExplicitName(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name     string
		explicit string
		projects []ProjectInfo
	}{
		{
			name:     "exact name",
			explicit: "Game",
			projects: []ProjectInfo{
				{ID: filepath.Join(root, "first"), Name: "Game"},
				{ID: filepath.Join(root, "second"), Name: "game"},
			},
		},
		{
			name:     "partial name",
			explicit: "Game",
			projects: []ProjectInfo{
				{ID: filepath.Join(root, "first"), Name: "Game Alpha"},
				{ID: filepath.Join(root, "second"), Name: "Game Beta"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project, offlinePath, err := ResolveProjectOrPath(test.explicit, t.TempDir(), test.projects)
			if err == nil {
				t.Fatal("expected ambiguity error")
			}
			if project != nil || offlinePath != "" {
				t.Fatalf("project = %#v, offlinePath = %q; want neither", project, offlinePath)
			}
		})
	}
}

func TestResolveProjectOrPathReturnsExplicitOfflineUnityProject(t *testing.T) {
	projectRoot := createBridgeTestUnityProject(t)
	nestedPath := filepath.Join(projectRoot, "Assets", "Scripts")
	if err := os.MkdirAll(nestedPath, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	project, offlinePath, err := ResolveProjectOrPath(nestedPath, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("ResolveProjectOrPath failed: %v", err)
	}
	if project != nil {
		t.Fatalf("project = %#v, want nil", project)
	}
	if offlinePath != projectRoot {
		t.Fatalf("offline path = %q, want %q", offlinePath, projectRoot)
	}
}

func TestResolveProjectOrPathRejectsMissingExplicitInsideCwdProject(t *testing.T) {
	cwdProject := createBridgeTestUnityProject(t)
	connected := []ProjectInfo{{ID: cwdProject, Name: "Connected"}}

	project, offlinePath, err := ResolveProjectOrPath("missing-project", cwdProject, connected)
	if err == nil {
		t.Fatal("expected project-not-found error")
	}
	if project != nil || offlinePath != "" {
		t.Fatalf("project = %#v, offline path = %q; want neither", project, offlinePath)
	}
}

func TestResolveProjectOrPathDoesNotFallThroughUnknownExplicit(t *testing.T) {
	projectRoot := createBridgeTestUnityProject(t)
	projects := []ProjectInfo{{ID: projectRoot, Name: "Connected"}}

	project, offlinePath, err := ResolveProjectOrPath("unknown", projectRoot, projects)
	if err == nil {
		t.Fatal("expected project-not-found error")
	}
	if project != nil || offlinePath != "" {
		t.Fatalf("project = %#v, offline path = %q; want neither", project, offlinePath)
	}
}

func TestResolveProjectOrPathUsesCwdUnityProject(t *testing.T) {
	projectRoot := createBridgeTestUnityProject(t)
	cwd := filepath.Join(projectRoot, "Assets", "Scripts")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("create cwd: %v", err)
	}

	t.Run("connected", func(t *testing.T) {
		projects := []ProjectInfo{{ID: projectRoot, Name: "Game"}}
		project, offlinePath, err := ResolveProjectOrPath("", cwd, projects)
		if err != nil {
			t.Fatalf("ResolveProjectOrPath failed: %v", err)
		}
		if project == nil || project.ID != projectRoot {
			t.Fatalf("project = %#v, want %q", project, projectRoot)
		}
		if offlinePath != "" {
			t.Fatalf("offline path = %q, want empty", offlinePath)
		}
	})

	t.Run("offline before ambiguous connections", func(t *testing.T) {
		otherRoot := t.TempDir()
		projects := []ProjectInfo{
			{ID: filepath.Join(otherRoot, "alpha"), Name: "Alpha"},
			{ID: filepath.Join(otherRoot, "beta"), Name: "Beta"},
		}
		project, offlinePath, err := ResolveProjectOrPath("", cwd, projects)
		if err != nil {
			t.Fatalf("ResolveProjectOrPath failed: %v", err)
		}
		if project != nil {
			t.Fatalf("project = %#v, want nil", project)
		}
		if offlinePath != projectRoot {
			t.Fatalf("offline path = %q, want %q", offlinePath, projectRoot)
		}
	})
}

func TestResolveProjectOrPathUsesCwdGitRoot(t *testing.T) {
	repositoryRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(repositoryRoot, ".git"), 0o755); err != nil {
		t.Fatalf("create .git: %v", err)
	}
	cwd := filepath.Join(repositoryRoot, "tools", "nested")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("create cwd: %v", err)
	}
	projects := []ProjectInfo{
		{ID: filepath.Join(repositoryRoot, "client"), Name: "Client", GitRoot: repositoryRoot},
		{ID: filepath.Join(t.TempDir(), "other"), Name: "Other", GitRoot: t.TempDir()},
	}

	project, offlinePath, err := ResolveProjectOrPath("", cwd, projects)
	if err != nil {
		t.Fatalf("ResolveProjectOrPath failed: %v", err)
	}
	if project == nil || project.ID != projects[0].ID {
		t.Fatalf("project = %#v, want %#v", project, projects[0])
	}
	if offlinePath != "" {
		t.Fatalf("offline path = %q, want empty", offlinePath)
	}
}

func TestResolveProjectOrPathFallsBackByConnectionCount(t *testing.T) {
	cwd := t.TempDir()
	project := ProjectInfo{ID: filepath.Join(t.TempDir(), "only"), Name: "Only"}

	t.Run("single", func(t *testing.T) {
		selected, offlinePath, err := ResolveProjectOrPath("", cwd, []ProjectInfo{project})
		if err != nil {
			t.Fatalf("ResolveProjectOrPath failed: %v", err)
		}
		if selected == nil || selected.ID != project.ID {
			t.Fatalf("project = %#v, want %#v", selected, project)
		}
		if offlinePath != "" {
			t.Fatalf("offline path = %q, want empty", offlinePath)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		projects := []ProjectInfo{project, {ID: filepath.Join(t.TempDir(), "second"), Name: "Second"}}
		selected, offlinePath, err := ResolveProjectOrPath("", cwd, projects)
		if err == nil {
			t.Fatal("expected multiple-project error")
		}
		if selected != nil || offlinePath != "" {
			t.Fatalf("project = %#v, offline path = %q; want neither", selected, offlinePath)
		}
	})

	t.Run("none", func(t *testing.T) {
		selected, offlinePath, err := ResolveProjectOrPath("", cwd, nil)
		if err != nil {
			t.Fatalf("ResolveProjectOrPath failed: %v", err)
		}
		if selected != nil || offlinePath != "" {
			t.Fatalf("project = %#v, offline path = %q; want neither", selected, offlinePath)
		}
	})
}

func TestResolveUnityProjectPathRequiresExistingStartPath(t *testing.T) {
	projectRoot := createBridgeTestUnityProject(t)
	nestedPath := filepath.Join(projectRoot, "Assets", "Scripts")
	if err := os.MkdirAll(nestedPath, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	if got := ResolveUnityProjectPath(nestedPath); got != projectRoot {
		t.Fatalf("resolved project = %q, want %q", got, projectRoot)
	}
	if got := ResolveUnityProjectPath(filepath.Join(projectRoot, "missing")); got != "" {
		t.Fatalf("missing start path resolved to %q", got)
	}
}

func TestMatchProjectIgnoresWindowsPathCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path comparison")
	}
	projectPath := filepath.Join(t.TempDir(), "MixedCase")
	projects := []ProjectInfo{{ID: strings.ToUpper(projectPath), Name: "Game"}}
	match := MatchProject(CwdHints{ProjectPath: strings.ToLower(projectPath)}, projects)
	if match == nil {
		t.Fatal("case-only path difference did not match")
	}
}

func TestNormalizePathPreservesFilesystemRoot(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(os.PathSeparator)
	if got := normalizePath(root); got == "" {
		t.Fatal("filesystem root normalized to empty")
	}
}

func createBridgeTestUnityProject(t *testing.T) string {
	t.Helper()
	projectRoot := t.TempDir()
	for _, directory := range []string{"Assets", "ProjectSettings"} {
		if err := os.MkdirAll(filepath.Join(projectRoot, directory), 0o755); err != nil {
			t.Fatalf("create %s: %v", directory, err)
		}
	}
	return projectRoot
}
