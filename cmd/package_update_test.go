package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neptaco/uniforge/pkg/bridge"
	"github.com/neptaco/uniforge/pkg/updater"
	"github.com/spf13/cobra"
)

func TestRewritePackageManifestVersion(t *testing.T) {
	tests := []struct {
		name          string
		manifest      string
		wantReference string
		wantErr       string
	}{
		{
			name: "replaces existing fragment",
			manifest: `{
  "dependencies": {
    "com.unity.test-framework": "1.1.33",
    "dev.crysta.uniforge": "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#main"
  }
}`,
			wantReference: "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#v0.12.0",
		},
		{
			name: "adds missing fragment",
			manifest: `{
  "dependencies": {
    "dev.crysta.uniforge": "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge"
  }
}`,
			wantReference: "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#v0.12.0",
		},
		{
			name: "rejects non-git reference",
			manifest: `{
  "dependencies": {
    "dev.crysta.uniforge": "file:../dev.crysta.uniforge"
  }
}`,
			wantErr: "git URL",
		},
		{
			name: "rejects registry reference",
			manifest: `{
  "dependencies": {
    "dev.crysta.uniforge": "0.11.0"
  }
}`,
			wantErr: "git URL",
		},
		{
			name: "rejects git URL without package path",
			manifest: `{
  "dependencies": {
    "dev.crysta.uniforge": "https://github.com/neptaco/uniforge-unity.git#v0.11.0"
  }
}`,
			wantErr: "git URL",
		},
		{
			name: "rejects missing package entry",
			manifest: `{
  "dependencies": {
    "com.unity.test-framework": "1.1.33"
  }
}`,
			wantErr: unityPackageID,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updated, err := rewritePackageManifestVersion([]byte(test.manifest), "0.12.0")
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("rewritePackageManifestVersion error = %v, want it to contain %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("rewritePackageManifestVersion failed: %v", err)
			}

			var manifest struct {
				Dependencies map[string]string `json:"dependencies"`
			}
			if err := json.Unmarshal(updated, &manifest); err != nil {
				t.Fatalf("decode updated manifest: %v", err)
			}
			if got := manifest.Dependencies[unityPackageID]; got != test.wantReference {
				t.Fatalf("package reference = %q, want %q", got, test.wantReference)
			}
			if strings.Contains(test.manifest, "com.unity.test-framework") &&
				manifest.Dependencies["com.unity.test-framework"] != "1.1.33" {
				t.Fatal("rewrite removed or changed a sibling dependency")
			}
		})
	}
}

func TestRemovePackageLockEntry(t *testing.T) {
	t.Run("removes existing entry", func(t *testing.T) {
		input := []byte(`{
  "dependencies": {
    "com.unity.test-framework": {"version": "1.1.33"},
    "dev.crysta.uniforge": {"version": "https://example.invalid#main"}
  }
}`)

		updated, changed, err := removePackageLockEntry(input)
		if err != nil {
			t.Fatalf("removePackageLockEntry failed: %v", err)
		}
		if !changed {
			t.Fatal("changed = false, want true")
		}

		var lock struct {
			Dependencies map[string]json.RawMessage `json:"dependencies"`
		}
		if err := json.Unmarshal(updated, &lock); err != nil {
			t.Fatalf("decode updated lock: %v", err)
		}
		if _, exists := lock.Dependencies[unityPackageID]; exists {
			t.Fatalf("%s still exists in lock", unityPackageID)
		}
		if _, exists := lock.Dependencies["com.unity.test-framework"]; !exists {
			t.Fatal("removePackageLockEntry removed a sibling dependency")
		}
	})

	t.Run("keeps lock without entry unchanged", func(t *testing.T) {
		input := []byte(`{"dependencies":{"com.unity.test-framework":{"version":"1.1.33"}}}`)

		updated, changed, err := removePackageLockEntry(input)
		if err != nil {
			t.Fatalf("removePackageLockEntry failed: %v", err)
		}
		if changed {
			t.Fatal("changed = true, want false")
		}
		if string(updated) != string(input) {
			t.Fatalf("unchanged lock was reformatted: got %q, want %q", updated, input)
		}
	})
}

func TestUpdateOfflinePackageFiles(t *testing.T) {
	t.Run("updates manifest and removes lock entry", func(t *testing.T) {
		projectPath := t.TempDir()
		packagesPath := filepath.Join(projectPath, "Packages")
		if err := os.MkdirAll(packagesPath, 0o755); err != nil {
			t.Fatal(err)
		}
		manifestPath := filepath.Join(packagesPath, "manifest.json")
		lockPath := filepath.Join(packagesPath, "packages-lock.json")
		manifest := []byte(`{
  "dependencies": {
    "com.unity.test-framework": "1.1.33",
    "dev.crysta.uniforge": "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#main"
  },
  "testables": ["com.example.tests"],
  "custom": {"keep": true}
}`)
		lock := []byte(`{
  "dependencies": {
    "com.unity.test-framework": {"version": "1.1.33", "depth": 0},
    "dev.crysta.uniforge": {"version": "https://example.invalid#main", "depth": 0}
  },
  "custom": {"keep": true}
}`)
		if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lockPath, lock, 0o640); err != nil {
			t.Fatal(err)
		}

		if err := updateOfflinePackageFiles(projectPath, "0.12.0"); err != nil {
			t.Fatalf("updateOfflinePackageFiles failed: %v", err)
		}

		updatedManifest := readJSONDocument(t, manifestPath)
		var dependencies map[string]string
		if err := json.Unmarshal(updatedManifest["dependencies"], &dependencies); err != nil {
			t.Fatal(err)
		}
		if got, want := dependencies[unityPackageID], "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#v0.12.0"; got != want {
			t.Fatalf("manifest package reference = %q, want %q", got, want)
		}
		var testables []string
		if err := json.Unmarshal(updatedManifest["testables"], &testables); err != nil {
			t.Fatal(err)
		}
		if len(testables) != 1 || testables[0] != "com.example.tests" {
			t.Fatalf("manifest testables changed: %#v", testables)
		}
		var manifestCustom struct {
			Keep bool `json:"keep"`
		}
		if err := json.Unmarshal(updatedManifest["custom"], &manifestCustom); err != nil {
			t.Fatal(err)
		}
		if !manifestCustom.Keep {
			t.Fatalf("manifest custom field changed: %s", updatedManifest["custom"])
		}

		updatedLock := readJSONDocument(t, lockPath)
		var lockDependencies map[string]json.RawMessage
		if err := json.Unmarshal(updatedLock["dependencies"], &lockDependencies); err != nil {
			t.Fatal(err)
		}
		if _, exists := lockDependencies[unityPackageID]; exists {
			t.Fatalf("%s still exists in package lock", unityPackageID)
		}
		var sibling struct {
			Version string `json:"version"`
			Depth   int    `json:"depth"`
		}
		if err := json.Unmarshal(lockDependencies["com.unity.test-framework"], &sibling); err != nil {
			t.Fatal(err)
		}
		if sibling.Version != "1.1.33" || sibling.Depth != 0 {
			t.Fatalf("lock sibling changed: %#v", sibling)
		}
		var lockCustom struct {
			Keep bool `json:"keep"`
		}
		if err := json.Unmarshal(updatedLock["custom"], &lockCustom); err != nil {
			t.Fatal(err)
		}
		if !lockCustom.Keep {
			t.Fatalf("lock custom field changed: %s", updatedLock["custom"])
		}

		assertFileMode(t, manifestPath, 0o600)
		assertFileMode(t, lockPath, 0o640)
	})

	t.Run("allows missing lock without creating it", func(t *testing.T) {
		projectPath, manifestPath := createOfflinePackageTestProject(t)
		lockPath := filepath.Join(projectPath, "Packages", "packages-lock.json")

		if err := updateOfflinePackageFiles(projectPath, "0.12.0"); err != nil {
			t.Fatalf("updateOfflinePackageFiles failed: %v", err)
		}
		if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
			t.Fatalf("packages-lock.json was created or stat returned an unexpected error: %v", err)
		}
		if !strings.Contains(string(mustReadFile(t, manifestPath)), "#v0.12.0") {
			t.Fatal("manifest was not updated")
		}
	})

	t.Run("leaves lock without package entry byte-identical", func(t *testing.T) {
		projectPath, _ := createOfflinePackageTestProject(t)
		lockPath := filepath.Join(projectPath, "Packages", "packages-lock.json")
		lock := []byte(`{"dependencies":{"com.unity.test-framework":{"version":"1.1.33"}}}`)
		if err := os.WriteFile(lockPath, lock, 0o644); err != nil {
			t.Fatal(err)
		}

		if err := updateOfflinePackageFiles(projectPath, "0.12.0"); err != nil {
			t.Fatalf("updateOfflinePackageFiles failed: %v", err)
		}
		if got := mustReadFile(t, lockPath); string(got) != string(lock) {
			t.Fatalf("unchanged lock was rewritten: got %q, want %q", got, lock)
		}
	})

	t.Run("malformed lock leaves manifest unchanged", func(t *testing.T) {
		projectPath, manifestPath := createOfflinePackageTestProject(t)
		originalManifest := mustReadFile(t, manifestPath)
		lockPath := filepath.Join(projectPath, "Packages", "packages-lock.json")
		if err := os.WriteFile(lockPath, []byte(`not json`), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := updateOfflinePackageFiles(projectPath, "0.12.0"); err == nil {
			t.Fatal("updateOfflinePackageFiles error = nil, want malformed lock error")
		}
		if got := mustReadFile(t, manifestPath); string(got) != string(originalManifest) {
			t.Fatalf("manifest changed after lock parse failure: got %q, want %q", got, originalManifest)
		}
	})
}

func TestPackageUpdateSourceUsesInstalledGitReference(t *testing.T) {
	projectPath, _ := createOfflinePackageTestProject(t)

	source, err := packageUpdateSource(projectPath)
	if err != nil {
		t.Fatalf("packageUpdateSource failed: %v", err)
	}
	if got, want := source.repositoryURL, "https://github.com/neptaco/uniforge-unity.git"; got != want {
		t.Fatalf("repository URL = %q, want %q", got, want)
	}
	if got, want := source.packagePath, "Packages/dev.crysta.uniforge"; got != want {
		t.Fatalf("package path = %q, want %q", got, want)
	}
}

func TestUpdateOfflinePackageChecksUnityCompatibilityBeforeWriting(t *testing.T) {
	projectPath, manifestPath := createOfflinePackageTestProject(t)
	projectVersionPath := filepath.Join(projectPath, "ProjectSettings", "ProjectVersion.txt")
	if err := os.WriteFile(projectVersionPath, []byte("m_EditorVersion: 2022.3.62f1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalManifest := mustReadFile(t, manifestPath)

	originalLoader := packageManifestLoader
	originalForce := packageUpdateForce
	t.Cleanup(func() {
		packageManifestLoader = originalLoader
		packageUpdateForce = originalForce
	})
	var loadedTag string
	packageManifestLoader = func(_ context.Context, source packageSource, tag string) ([]byte, error) {
		loadedTag = tag
		if got, want := source.packagePath, "Packages/dev.crysta.uniforge"; got != want {
			t.Fatalf("package path = %q, want %q", got, want)
		}
		return []byte(`{"name":"dev.crysta.uniforge","unity":"6000.0","unityRelease":"0f1"}`), nil
	}
	packageUpdateForce = false
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())

	err := updateOfflinePackage(cmd, projectPath, "0.12.0")
	if err == nil || !strings.Contains(err.Error(), "requires Unity 6000.0.0f1 or later") {
		t.Fatalf("updateOfflinePackage error = %v, want Unity compatibility error", err)
	}
	if loadedTag != "v0.12.0" {
		t.Fatalf("loaded tag = %q, want %q", loadedTag, "v0.12.0")
	}
	if got := mustReadFile(t, manifestPath); string(got) != string(originalManifest) {
		t.Fatalf("manifest changed after compatibility failure: got %q, want %q", got, originalManifest)
	}
}

func TestUpdateOfflinePackageForceBypassesCompatibilityFailure(t *testing.T) {
	projectPath, manifestPath := createOfflinePackageTestProject(t)
	projectVersionPath := filepath.Join(projectPath, "ProjectSettings", "ProjectVersion.txt")
	if err := os.WriteFile(projectVersionPath, []byte("m_EditorVersion: 2022.3.62f1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	originalLoader := packageManifestLoader
	originalForce := packageUpdateForce
	t.Cleanup(func() {
		packageManifestLoader = originalLoader
		packageUpdateForce = originalForce
	})
	packageManifestLoader = func(context.Context, packageSource, string) ([]byte, error) {
		return []byte(`{"name":"dev.crysta.uniforge","unity":"6000.0","unityRelease":"0f1"}`), nil
	}
	packageUpdateForce = true
	var stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetErr(&stderr)

	if err := updateOfflinePackage(cmd, projectPath, "0.12.0"); err != nil {
		t.Fatalf("updateOfflinePackage failed with --force: %v", err)
	}
	if !strings.Contains(stderr.String(), "continuing because --force was set") {
		t.Fatalf("warning = %q, want --force explanation", stderr.String())
	}
	if !strings.Contains(string(mustReadFile(t, manifestPath)), "#v0.12.0") {
		t.Fatal("manifest was not updated with --force")
	}
}

func TestResolvePackageUpdateVersionUsesCachedVersion(t *testing.T) {
	deps := packageVersionResolverDeps{
		prepare: func(updater.AutoCheckOptions) (updater.AutoCheckDecision, error) {
			return updater.AutoCheckDecision{LatestVersion: "0.12.0"}, nil
		},
		refresh: func(context.Context, updater.AutoCheckOptions) error {
			return errors.New("refresh must not be called for a cache hit")
		},
		read: func(string) (string, error) {
			return "", errors.New("read must not be called for a cache hit")
		},
	}

	got, err := resolvePackageUpdateVersion(context.Background(), "", updater.AutoCheckOptions{}, deps)
	if err != nil {
		t.Fatalf("resolvePackageUpdateVersion failed: %v", err)
	}
	if got != "0.12.0" {
		t.Fatalf("version = %q, want %q", got, "0.12.0")
	}
}

func TestResolvePackageUpdateVersionRefreshesEmptyCache(t *testing.T) {
	refreshed := false
	deps := packageVersionResolverDeps{
		prepare: func(updater.AutoCheckOptions) (updater.AutoCheckDecision, error) {
			return updater.AutoCheckDecision{}, nil
		},
		refresh: func(context.Context, updater.AutoCheckOptions) error {
			refreshed = true
			return nil
		},
		read: func(string) (string, error) {
			if !refreshed {
				return "", errors.New("cache read before refresh")
			}
			return "0.12.0", nil
		},
	}

	got, err := resolvePackageUpdateVersion(context.Background(), "", updater.AutoCheckOptions{}, deps)
	if err != nil {
		t.Fatalf("resolvePackageUpdateVersion failed: %v", err)
	}
	if got != "0.12.0" {
		t.Fatalf("version = %q, want %q", got, "0.12.0")
	}
}

func TestResolvePackageUpdateVersionUsesCachePopulatedDuringFailedRefresh(t *testing.T) {
	deps := packageVersionResolverDeps{
		prepare: func(updater.AutoCheckOptions) (updater.AutoCheckDecision, error) {
			return updater.AutoCheckDecision{}, nil
		},
		refresh: func(context.Context, updater.AutoCheckOptions) error {
			return errors.New("refresh timed out")
		},
		read: func(string) (string, error) {
			return "0.12.0", nil
		},
	}

	got, err := resolvePackageUpdateVersion(context.Background(), "", updater.AutoCheckOptions{}, deps)
	if err != nil {
		t.Fatalf("resolvePackageUpdateVersion failed: %v", err)
	}
	if got != "0.12.0" {
		t.Fatalf("version = %q, want %q", got, "0.12.0")
	}
}

func TestResolvePackageUpdateVersionRefreshUsesTenSecondTimeout(t *testing.T) {
	deadlineIsTenSeconds := false
	deps := packageVersionResolverDeps{
		prepare: func(updater.AutoCheckOptions) (updater.AutoCheckDecision, error) {
			return updater.AutoCheckDecision{}, nil
		},
		refresh: func(ctx context.Context, _ updater.AutoCheckOptions) error {
			deadline, ok := ctx.Deadline()
			if ok {
				remaining := time.Until(deadline)
				deadlineIsTenSeconds = remaining >= 9*time.Second && remaining <= 10*time.Second
			}
			return nil
		},
		read: func(string) (string, error) { return "0.12.0", nil },
	}

	if _, err := resolvePackageUpdateVersion(context.Background(), "", updater.AutoCheckOptions{}, deps); err != nil {
		t.Fatalf("resolvePackageUpdateVersion failed: %v", err)
	}
	if !deadlineIsTenSeconds {
		t.Fatal("refresh context did not have the required 10 second timeout")
	}
}

func TestResolvePackageUpdateVersionRequiresExplicitVersionWhenLatestIsUnknown(t *testing.T) {
	deps := packageVersionResolverDeps{
		prepare: func(updater.AutoCheckOptions) (updater.AutoCheckDecision, error) {
			return updater.AutoCheckDecision{}, nil
		},
		refresh: func(context.Context, updater.AutoCheckOptions) error { return nil },
		read:    func(string) (string, error) { return "", nil },
	}

	_, err := resolvePackageUpdateVersion(context.Background(), "", updater.AutoCheckOptions{}, deps)
	if err == nil || !strings.Contains(err.Error(), "--version") {
		t.Fatalf("resolvePackageUpdateVersion error = %v, want --version guidance", err)
	}
}

func TestResolvePackageUpdateVersionUsesExplicitVersionWithoutCache(t *testing.T) {
	deps := packageVersionResolverDeps{
		prepare: func(updater.AutoCheckOptions) (updater.AutoCheckDecision, error) {
			return updater.AutoCheckDecision{}, errors.New("prepare must not be called")
		},
		refresh: func(context.Context, updater.AutoCheckOptions) error {
			return errors.New("refresh must not be called")
		},
		read: func(string) (string, error) {
			return "", errors.New("read must not be called")
		},
	}

	got, err := resolvePackageUpdateVersion(context.Background(), "0.11.1", updater.AutoCheckOptions{}, deps)
	if err != nil {
		t.Fatalf("resolvePackageUpdateVersion failed: %v", err)
	}
	if got != "0.11.1" {
		t.Fatalf("version = %q, want %q", got, "0.11.1")
	}
}

func TestPackageUpdateReachedTargetVersion(t *testing.T) {
	tests := []struct {
		name     string
		projects []bridge.ProjectInfo
		want     bool
	}{
		{
			name: "target project reached version",
			projects: []bridge.ProjectInfo{
				{ID: "/projects/game", PackageVersion: "0.12.0"},
			},
			want: true,
		},
		{
			name: "target project still stale",
			projects: []bridge.ProjectInfo{
				{ID: "/projects/game", PackageVersion: "0.11.0"},
			},
		},
		{name: "target project temporarily absent"},
		{
			name: "different project reached version",
			projects: []bridge.ProjectInfo{
				{ID: "/projects/other", PackageVersion: "0.12.0"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := packageUpdateReached(test.projects, "/projects/game", "0.12.0"); got != test.want {
				t.Fatalf("packageUpdateReached = %t, want %t", got, test.want)
			}
		})
	}
}

func TestWaitForPackageUpdateRejectsVersionObservedAfterDeadline(t *testing.T) {
	lister := delayedPackageProjectLister{
		delay: 15 * time.Millisecond,
		projects: []bridge.ProjectInfo{
			{ID: "/projects/game", PackageVersion: "0.12.0"},
		},
	}

	reached, err := waitForPackageUpdate(
		context.Background(),
		lister,
		"/projects/game",
		"0.12.0",
		time.Millisecond,
		5*time.Millisecond,
	)
	if err != nil {
		t.Fatalf("waitForPackageUpdate failed: %v", err)
	}
	if reached {
		t.Fatal("waitForPackageUpdate accepted a version observed after the deadline")
	}
}

func TestPackageUpdateCommandSurface(t *testing.T) {
	if packageCmd.Parent() != rootCmd {
		t.Fatalf("package parent = %v, want root command", packageCmd.Parent())
	}
	if packageCmd.Hidden {
		t.Fatal("package command must be visible")
	}
	if packageUpdateCmd.Parent() != packageCmd {
		t.Fatalf("package update parent = %v, want package command", packageUpdateCmd.Parent())
	}
	if packageUpdateCmd.Use != "update [project]" {
		t.Fatalf("package update use = %q", packageUpdateCmd.Use)
	}
	if packageUpdateCmd.Flags().Lookup("version") == nil ||
		packageUpdateCmd.Flags().Lookup("no-wait") == nil ||
		packageUpdateCmd.Flags().Lookup("force") == nil {
		t.Fatal("package update command is missing version, no-wait, or force flag")
	}
}

type delayedPackageProjectLister struct {
	delay    time.Duration
	projects []bridge.ProjectInfo
}

func (lister delayedPackageProjectLister) ListProjects(bool) (*bridge.ClientListProjectsResult, error) {
	time.Sleep(lister.delay)
	return &bridge.ClientListProjectsResult{Projects: lister.projects}, nil
}

func createOfflinePackageTestProject(t *testing.T) (projectPath, manifestPath string) {
	t.Helper()
	projectPath = t.TempDir()
	packagesPath := filepath.Join(projectPath, "Packages")
	projectSettingsPath := filepath.Join(projectPath, "ProjectSettings")
	if err := os.MkdirAll(packagesPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectPath, "Assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectSettingsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(projectSettingsPath, "ProjectVersion.txt"),
		[]byte("m_EditorVersion: 6000.5.0f1\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	manifestPath = filepath.Join(packagesPath, "manifest.json")
	manifest := []byte(`{
  "dependencies": {
    "dev.crysta.uniforge": "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#main"
  }
}`)
	if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
		t.Fatal(err)
	}
	return projectPath, manifestPath
}

func readJSONDocument(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(mustReadFile(t, path), &document); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return document
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Windows has no Unix permission bits; Go reports files as 666/444.
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %o, want %o", path, got, want)
	}
}
