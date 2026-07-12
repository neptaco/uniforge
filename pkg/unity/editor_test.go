package unity

import (
	"errors"
	"testing"
	"time"
)

func TestFindUnityProcessFromPSOutputMatchesProjectPathSafely(t *testing.T) {
	projectPath := "/Projects/O'Brien/Test Project"
	output := `
123 /Applications/Unity/Hub/Editor/2022.3.60f1/Unity.app/Contents/MacOS/Unity -projectPath /Projects/Other
456 /Applications/Unity/Hub/Editor/2022.3.60f1/Unity.app/Contents/MacOS/Unity -projectPath /Projects/O'Brien/Test Project
`

	pid := findUnityProcessFromPSOutput(output, projectPath)
	if pid != 456 {
		t.Fatalf("pid = %d, want 456", pid)
	}
}

func TestFindUnityProcessFromPSOutputIgnoresNonUnityProcesses(t *testing.T) {
	projectPath := "/Projects/MyGame"
	output := `
111 /usr/bin/python worker.py /Projects/MyGame
222 /Applications/Unity/Hub/Unity Hub.app/Contents/MacOS/Unity Hub
333 /Applications/Unity/Hub/Editor/2022.3.60f1/Unity.app/Contents/MacOS/Unity -projectPath /Projects/OtherGame
`

	pid := findUnityProcessFromPSOutput(output, projectPath)
	if pid != 0 {
		t.Fatalf("pid = %d, want 0", pid)
	}
}

func TestWaitForUnityProcessExitReturnsWhenProcessDisappears(t *testing.T) {
	callCount := 0
	findProcess := func(projectPath string) (int, error) {
		callCount++
		if callCount < 3 {
			return 1234, nil
		}
		return 0, nil
	}

	err := waitForUnityProcessExit("/Projects/MyGame", 50*time.Millisecond, time.Millisecond, findProcess)
	if err != nil {
		t.Fatalf("waitForUnityProcessExit failed: %v", err)
	}
}

func TestWaitForUnityProcessExitTimesOut(t *testing.T) {
	findProcess := func(projectPath string) (int, error) {
		return 1234, nil
	}

	err := waitForUnityProcessExit("/Projects/MyGame", 5*time.Millisecond, time.Millisecond, findProcess)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestWaitForUnityProcessExitPropagatesFinderErrors(t *testing.T) {
	findProcess := func(projectPath string) (int, error) {
		return 0, errors.New("ps failed")
	}

	err := waitForUnityProcessExit("/Projects/MyGame", time.Second, time.Millisecond, findProcess)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveEditorExecutablePathHandlesLinuxExecutablePath(t *testing.T) {
	path := "/opt/Unity/Hub/Editor/2022.3.60f1/Editor/Unity"

	resolved := resolveEditorExecutablePath("linux", path)
	if resolved != path {
		t.Fatalf("resolved = %q, want %q", resolved, path)
	}
}

func TestResolveEditorExecutablePathBuildsLinuxExecutableFromVersionDir(t *testing.T) {
	baseDir := "/opt/Unity/Hub/Editor/2022.3.60f1"

	resolved := resolveEditorExecutablePath("linux", baseDir)
	expected := "/opt/Unity/Hub/Editor/2022.3.60f1/Editor/Unity"
	if resolved != expected {
		t.Fatalf("resolved = %q, want %q", resolved, expected)
	}
}
