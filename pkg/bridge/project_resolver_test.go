package bridge

import "testing"

func TestMatchProjectPrefersProjectPath(t *testing.T) {
	projects := []ProjectInfo{
		{ID: "/repo/alpha", Name: "Alpha", GitRoot: "/repo"},
		{ID: "/repo/beta", Name: "Beta", GitRoot: "/repo"},
	}

	match := MatchProject(CwdHints{
		ProjectPath: "/repo/beta",
		GitRoot:     "/repo",
	}, projects)

	if match == nil || match.ID != "/repo/beta" {
		t.Fatalf("expected /repo/beta, got %#v", match)
	}
}

func TestMatchProjectRequiresUniqueGitRoot(t *testing.T) {
	projects := []ProjectInfo{
		{ID: "/repo/alpha", Name: "Alpha", GitRoot: "/repo"},
		{ID: "/repo/beta", Name: "Beta", GitRoot: "/repo"},
	}

	match := MatchProject(CwdHints{GitRoot: "/repo"}, projects)
	if match != nil {
		t.Fatalf("expected nil for ambiguous git root, got %#v", match)
	}
}

func TestResolveProjectMatchesExplicitName(t *testing.T) {
	projects := []ProjectInfo{
		{ID: "/repo/alpha", Name: "Alpha"},
	}

	project, err := ResolveProject("alpha", CwdHints{}, projects)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project == nil || project.ID != "/repo/alpha" {
		t.Fatalf("unexpected project: %#v", project)
	}
}
