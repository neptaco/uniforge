package unity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanUnityProjectRequiresTarget(t *testing.T) {
	_, err := CleanUnityProject(CleanOptions{ProjectPath: t.TempDir()})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCleanUnityProjectLockfileMissing(t *testing.T) {
	projectPath := t.TempDir()

	result, err := CleanUnityProject(CleanOptions{
		ProjectPath: projectPath,
		Targets:     []CleanTarget{CleanTargetLockfile},
		findUnityProcess: func(string) (int, error) {
			t.Fatal("findUnityProcess should not be called when lockfile is missing")
			return 0, nil
		},
	})
	if err != nil {
		t.Fatalf("CleanUnityProject failed: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(result.Items))
	}
	if result.Items[0].Status != CleanItemMissing {
		t.Fatalf("status = %s, want %s", result.Items[0].Status, CleanItemMissing)
	}
}

func TestCleanUnityProjectLockfileDryRun(t *testing.T) {
	projectPath, lockfile := createUnityLockfile(t)

	result, err := CleanUnityProject(CleanOptions{
		ProjectPath: projectPath,
		Targets:     []CleanTarget{CleanTargetLockfile},
		DryRun:      true,
		findUnityProcess: func(gotProjectPath string) (int, error) {
			if gotProjectPath != projectPath {
				t.Fatalf("projectPath = %q, want %q", gotProjectPath, projectPath)
			}
			return 0, nil
		},
	})
	if err != nil {
		t.Fatalf("CleanUnityProject failed: %v", err)
	}
	if result.Items[0].Status != CleanItemWouldClean {
		t.Fatalf("status = %s, want %s", result.Items[0].Status, CleanItemWouldClean)
	}
	if _, err := os.Stat(lockfile); err != nil {
		t.Fatalf("lockfile should remain after dry run: %v", err)
	}
}

func TestCleanUnityProjectLockfileRemove(t *testing.T) {
	projectPath, lockfile := createUnityLockfile(t)

	result, err := CleanUnityProject(CleanOptions{
		ProjectPath: projectPath,
		Targets:     []CleanTarget{CleanTargetLockfile},
		findUnityProcess: func(string) (int, error) {
			return 0, nil
		},
	})
	if err != nil {
		t.Fatalf("CleanUnityProject failed: %v", err)
	}
	if result.Items[0].Status != CleanItemRemoved {
		t.Fatalf("status = %s, want %s", result.Items[0].Status, CleanItemRemoved)
	}
	if _, err := os.Stat(lockfile); !os.IsNotExist(err) {
		t.Fatalf("lockfile should be removed, stat err = %v", err)
	}
}

func TestCleanUnityProjectLockfileSkipsWhenUnityRunning(t *testing.T) {
	projectPath, lockfile := createUnityLockfile(t)

	result, err := CleanUnityProject(CleanOptions{
		ProjectPath: projectPath,
		Targets:     []CleanTarget{CleanTargetLockfile},
		findUnityProcess: func(string) (int, error) {
			return 1234, nil
		},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result.Items[0].Status != CleanItemSkipped {
		t.Fatalf("status = %s, want %s", result.Items[0].Status, CleanItemSkipped)
	}
	if _, err := os.Stat(lockfile); err != nil {
		t.Fatalf("lockfile should remain when Unity is running: %v", err)
	}
}

func createUnityLockfile(t *testing.T) (string, string) {
	t.Helper()

	projectPath := t.TempDir()
	tempDir := filepath.Join(projectPath, "Temp")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		t.Fatalf("create Temp dir: %v", err)
	}
	lockfile := filepath.Join(tempDir, "UnityLockfile")
	if err := os.WriteFile(lockfile, []byte("lock"), 0o644); err != nil {
		t.Fatalf("create lockfile: %v", err)
	}
	return projectPath, lockfile
}
