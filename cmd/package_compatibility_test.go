package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestParseProjectUnityVersion(t *testing.T) {
	version, display, err := parseProjectUnityVersion([]byte(
		"m_EditorVersion: 6000.0.70f1\nm_EditorVersionWithRevision: ignored\n",
	))
	if err != nil {
		t.Fatal(err)
	}
	if display != "6000.0.70f1" || version.major != 6000 || version.minor != 0 || version.update != 70 {
		t.Fatalf("version = %#v, display = %q", version, display)
	}

	for _, test := range []struct {
		name string
		data string
	}{
		{name: "missing version", data: "other: value\n"},
		{name: "invalid version", data: "m_EditorVersion: latest\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := parseProjectUnityVersion([]byte(test.data)); err == nil {
				t.Fatal("parseProjectUnityVersion succeeded")
			}
		})
	}
}

func TestParsePackageMinimumUnity(t *testing.T) {
	tests := []struct {
		name        string
		unity       string
		release     string
		wantDisplay string
		wantNil     bool
		wantErr     string
	}{
		{name: "no declared minimum", wantNil: true},
		{name: "major and minor", unity: "6000.0", wantDisplay: "6000.0"},
		{name: "specific release", unity: "6000.0", release: "70f1", wantDisplay: "6000.0.70f1"},
		{name: "invalid unity", unity: "Unity 6", wantErr: "major.minor"},
		{name: "invalid release", unity: "6000.0", release: "70", wantErr: "unityRelease"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version, display, err := parsePackageMinimumUnity(test.unity, test.release)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (version == nil) != test.wantNil || display != test.wantDisplay {
				t.Fatalf("version = %#v, display = %q", version, display)
			}
		})
	}
}

func TestUnityCompatibilityComparison(t *testing.T) {
	project, err := parseUnityFullVersion("6000.0.70f1")
	if err != nil {
		t.Fatal(err)
	}

	compatible, _, err := parsePackageMinimumUnity("6000.0", "69f1")
	if err != nil {
		t.Fatal(err)
	}
	incompatible, _, err := parsePackageMinimumUnity("6000.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if compareUnityVersions(project, *compatible) <= 0 {
		t.Fatal("newer project version was not considered compatible")
	}
	if compareUnityVersions(project, *incompatible) >= 0 {
		t.Fatal("older project version was considered compatible")
	}

	minimumStable, _, err := parsePackageMinimumUnity("6000.0", "0f1")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"6000.6.0b4", "7000.0.0b1"} {
		futureVersion, err := parseUnityFullVersion(value)
		if err != nil {
			t.Fatal(err)
		}
		if compareUnityVersions(futureVersion, *minimumStable) <= 0 {
			t.Fatalf("newer Unity stream %s was not considered compatible", value)
		}
	}
	olderBeta, err := parseUnityFullVersion("6000.0.0b1")
	if err != nil {
		t.Fatal(err)
	}
	if compareUnityVersions(olderBeta, *minimumStable) >= 0 {
		t.Fatal("beta below the declared stable minimum was considered compatible")
	}
}

func TestInspectPackageCompatibility(t *testing.T) {
	projectPath := t.TempDir()
	projectSettingsPath := filepath.Join(projectPath, "ProjectSettings")
	if err := os.MkdirAll(projectSettingsPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(projectSettingsPath, "ProjectVersion.txt"),
		[]byte("m_EditorVersion: 6000.0.70f1\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	originalLoader := packageManifestLoader
	t.Cleanup(func() { packageManifestLoader = originalLoader })
	source := packageSource{packageID: "folder-name"}

	t.Run("uses the package manifest name and accepts a supported project", func(t *testing.T) {
		packageManifestLoader = func(context.Context, packageSource, string) ([]byte, error) {
			return []byte(`{"name":"com.example.real-name","unity":"6000.0"}`), nil
		}

		resolved, compatibility, err := inspectPackageCompatibility(
			context.Background(), projectPath, source, "v1.0.0",
		)
		if err != nil {
			t.Fatal(err)
		}
		if resolved.packageID != "com.example.real-name" {
			t.Fatalf("package ID = %q", resolved.packageID)
		}
		if compatibility.projectVersion != "6000.0.70f1" || compatibility.minimumUnity != "6000.0" {
			t.Fatalf("compatibility = %#v", compatibility)
		}
	})

	t.Run("rejects a project below the package minimum", func(t *testing.T) {
		packageManifestLoader = func(context.Context, packageSource, string) ([]byte, error) {
			return []byte(`{"name":"com.example.package","unity":"6000.1"}`), nil
		}

		_, compatibility, err := inspectPackageCompatibility(
			context.Background(), projectPath, source, "v1.0.0",
		)
		if err == nil || !strings.Contains(err.Error(), "requires Unity 6000.1") {
			t.Fatalf("error = %v", err)
		}
		if compatibility.projectVersion != "6000.0.70f1" || compatibility.minimumUnity != "6000.1" {
			t.Fatalf("compatibility = %#v", compatibility)
		}
	})

	t.Run("accepts a package without a declared minimum", func(t *testing.T) {
		packageManifestLoader = func(context.Context, packageSource, string) ([]byte, error) {
			return []byte(`{"name":"com.example.package"}`), nil
		}

		_, compatibility, err := inspectPackageCompatibility(
			context.Background(), projectPath, source, "v1.0.0",
		)
		if err != nil {
			t.Fatal(err)
		}
		if compatibility.packageDisplay() != "no minimum declared" {
			t.Fatalf("package display = %q", compatibility.packageDisplay())
		}
	})

	t.Run("accepts a newer beta stream", func(t *testing.T) {
		if err := os.WriteFile(
			filepath.Join(projectSettingsPath, "ProjectVersion.txt"),
			[]byte("m_EditorVersion: 6000.6.0b4\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		packageManifestLoader = func(context.Context, packageSource, string) ([]byte, error) {
			return []byte(`{"name":"dev.crysta.uniforge","unity":"6000.0","unityRelease":"0f1"}`), nil
		}

		_, compatibility, err := inspectPackageCompatibility(
			context.Background(), projectPath, source, "v1.0.0",
		)
		if err != nil {
			t.Fatal(err)
		}
		if compatibility.packageDisplay() != "6000.0.0f1 or later" {
			t.Fatalf("package display = %q", compatibility.packageDisplay())
		}
	})
}

func TestLoadPackageManifestFromGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	repositoryPath := t.TempDir()
	packagePath := filepath.Join(repositoryPath, "Packages", "com.example.package")
	if err := os.MkdirAll(packagePath, 0o755); err != nil {
		t.Fatal(err)
	}
	want := []byte(`{"name":"com.example.package","unity":"6000.0"}`)
	if err := os.WriteFile(filepath.Join(packagePath, "package.json"), want, 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repositoryPath, "init", "--quiet")
	runGitTestCommand(t, repositoryPath, "add", "Packages/com.example.package/package.json")
	runGitTestCommand(
		t,
		repositoryPath,
		"-c", "user.name=UniForge Test",
		"-c", "user.email=uniforge@example.invalid",
		"commit", "--quiet", "-m", "test package",
	)
	runGitTestCommand(t, repositoryPath, "tag", "v1.2.3")

	got, err := loadPackageManifestFromGit(context.Background(), packageSource{
		repositoryURL: repositoryPath,
		packagePath:   "Packages/com.example.package",
	}, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != string(want) {
		t.Fatalf("package manifest = %q, want %q", got, want)
	}
}

func TestRunPackageAddCompatibilityGate(t *testing.T) {
	projectPath := t.TempDir()
	for _, directory := range []string{"Assets", "Packages", "ProjectSettings"} {
		if err := os.MkdirAll(filepath.Join(projectPath, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath := filepath.Join(projectPath, "Packages", "manifest.json")
	originalManifest := []byte(`{"dependencies":{}}`)
	if err := os.WriteFile(manifestPath, originalManifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(projectPath, "ProjectSettings", "ProjectVersion.txt"),
		[]byte("m_EditorVersion: 2022.3.60f1\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	originalLoader := packageManifestLoader
	originalTag := packageAddTag
	originalForce := packageAddForce
	t.Cleanup(func() {
		packageManifestLoader = originalLoader
		packageAddTag = originalTag
		packageAddForce = originalForce
	})
	packageManifestLoader = func(context.Context, packageSource, string) ([]byte, error) {
		return []byte(`{"name":"dev.crysta.uniforge","unity":"6000.0","unityRelease":"0f1"}`), nil
	}
	packageAddTag = "v1.0.0"
	packageAddForce = false
	command := &cobra.Command{}
	command.SetContext(context.Background())
	command.SetOut(&strings.Builder{})

	err := runPackageAdd(
		command,
		[]string{projectPath, "example/package/Packages/dev.crysta.uniforge"},
	)
	if err == nil || !strings.Contains(err.Error(), "requires Unity 6000.0.0f1") {
		t.Fatalf("error = %v", err)
	}
	if got := mustReadFile(t, manifestPath); string(got) != string(originalManifest) {
		t.Fatalf("manifest changed after rejected installation: %s", got)
	}

	packageAddForce = true
	command.SetOut(&strings.Builder{})
	if err := runPackageAdd(
		command,
		[]string{projectPath, "example/package/Packages/dev.crysta.uniforge"},
	); err != nil {
		t.Fatalf("forced package add failed: %v", err)
	}
	manifest := readJSONDocument(t, manifestPath)
	if !strings.Contains(string(manifest["dependencies"]), "dev.crysta.uniforge") {
		t.Fatalf("forced package was not added: %s", manifest["dependencies"])
	}
}

func TestForcedCompatibilitySummary(t *testing.T) {
	compatibility := packageCompatibility{
		forced:  true,
		warning: "project version is unknown",
	}
	if got := compatibility.summary(); !strings.Contains(got, "forced") || !strings.Contains(got, compatibility.warning) {
		t.Fatalf("summary = %q", got)
	}
}

func runGitTestCommand(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %s: %v", strings.Join(arguments, " "), output, err)
	}
}
