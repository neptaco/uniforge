package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestParsePackageSource(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		wantID      string
		wantURL     string
		wantAPIBase string
		wantErr     string
	}{
		{
			name:        "expands short GitHub source",
			value:       "neptaco/uniforge-unity/Packages/dev.crysta.uniforge",
			wantID:      "dev.crysta.uniforge",
			wantURL:     "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge",
			wantAPIBase: "https://api.github.com/repos/neptaco/uniforge-unity",
		},
		{
			name:        "accepts github prefix",
			value:       "github:neptaco/uniforge-unity/Packages/dev.crysta.uniforge",
			wantID:      "dev.crysta.uniforge",
			wantURL:     "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge",
			wantAPIBase: "https://api.github.com/repos/neptaco/uniforge-unity",
		},
		{
			name:        "accepts full HTTPS GitHub URL",
			value:       "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge",
			wantID:      "dev.crysta.uniforge",
			wantURL:     "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge",
			wantAPIBase: "https://api.github.com/repos/neptaco/uniforge-unity",
		},
		{
			name:    "requires package path in shorthand",
			value:   "neptaco/uniforge-unity",
			wantErr: "owner/repository/path/to/package",
		},
		{
			name:    "requires path query in full URL",
			value:   "https://github.com/neptaco/uniforge-unity.git",
			wantErr: "?path=",
		},
		{
			name:    "keeps tag separate",
			value:   "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge#v0.11.0",
			wantErr: "--tag",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := parsePackageSource(test.value)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePackageSource failed: %v", err)
			}
			if source.packageID != test.wantID || source.manifestURL != test.wantURL || source.githubAPIBase != test.wantAPIBase {
				t.Fatalf("source = %#v", source)
			}
		})
	}
}

func TestResolvePackageAddTag(t *testing.T) {
	t.Run("uses explicit tag without GitHub lookup", func(t *testing.T) {
		got, err := resolvePackageAddTag(context.Background(), packageSource{}, "v0.11.0")
		if err != nil {
			t.Fatal(err)
		}
		if got != "v0.11.0" {
			t.Fatalf("tag = %q, want v0.11.0", got)
		}
	})

	t.Run("resolves highest semantic-version GitHub tag", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/tags" || r.URL.Query().Get("per_page") != "100" {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte(`[{"name":"v0.9.0"},{"name":"preview"},{"name":"v0.12.0"}]`))
		}))
		defer server.Close()

		got, err := resolvePackageAddTag(context.Background(), packageSource{githubAPIBase: server.URL}, "")
		if err != nil {
			t.Fatal(err)
		}
		if got != "v0.12.0" {
			t.Fatalf("tag = %q, want v0.12.0", got)
		}
	})

	t.Run("requires tag for a non-GitHub URL", func(t *testing.T) {
		_, err := resolvePackageAddTag(
			context.Background(),
			packageSource{originalDisplay: "https://git.example.com/team/package.git?path=Packages/example"},
			"",
		)
		if err == nil || !strings.Contains(err.Error(), "specify --tag") {
			t.Fatalf("error = %v, want --tag guidance", err)
		}
	})
}

func TestRewritePackageManifestForAdd(t *testing.T) {
	source := packageSource{
		packageID:   "dev.crysta.uniforge",
		manifestURL: "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge",
	}

	t.Run("adds only the requested package and preserves the manifest", func(t *testing.T) {
		input := []byte(`{
  "dependencies": {
    "com.unity.test-framework": "1.1.33"
  },
  "testables": ["com.example.tests"],
  "custom": {"keep": true}
}`)

		updated, err := rewritePackageManifestForAdd(input, source, "v0.12.0")
		if err != nil {
			t.Fatalf("rewritePackageManifestForAdd failed: %v", err)
		}

		var manifest struct {
			Dependencies map[string]string `json:"dependencies"`
			Testables    []string          `json:"testables"`
			Custom       struct {
				Keep bool `json:"keep"`
			} `json:"custom"`
		}
		if err := json.Unmarshal(updated, &manifest); err != nil {
			t.Fatalf("decode updated manifest: %v", err)
		}
		wantReference := source.manifestURL + "#v0.12.0"
		if got := manifest.Dependencies[source.packageID]; got != wantReference {
			t.Fatalf("package reference = %q, want %q", got, wantReference)
		}
		if _, exists := manifest.Dependencies["com.unity.ugui"]; exists {
			t.Fatal("an unrequested package was added")
		}
		if got := manifest.Dependencies["com.unity.test-framework"]; got != "1.1.33" {
			t.Fatalf("sibling dependency = %q, want 1.1.33", got)
		}
		if len(manifest.Testables) != 1 || manifest.Testables[0] != "com.example.tests" || !manifest.Custom.Keep {
			t.Fatalf("top-level fields changed: %#v", manifest)
		}
	})

	t.Run("rejects an existing package", func(t *testing.T) {
		input := []byte(`{"dependencies":{"dev.crysta.uniforge":"https://example.invalid"}}`)
		_, err := rewritePackageManifestForAdd(input, source, "v0.12.0")
		if err == nil || !strings.Contains(err.Error(), "already present") {
			t.Fatalf("error = %v, want existing-package guidance", err)
		}
	})
}

func TestAddPackageToProject(t *testing.T) {
	projectPath := t.TempDir()
	packagesPath := filepath.Join(projectPath, "Packages")
	if err := os.MkdirAll(packagesPath, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(packagesPath, "manifest.json")
	lockPath := filepath.Join(packagesPath, "packages-lock.json")
	if err := os.WriteFile(manifestPath, []byte(`{"dependencies":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	lock := []byte(`{"leave":"unchanged"}`)
	if err := os.WriteFile(lockPath, lock, 0o640); err != nil {
		t.Fatal(err)
	}
	source := packageSource{
		packageID:   "dev.crysta.uniforge",
		manifestURL: "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge",
	}

	if err := addPackageToProject(projectPath, source, "v0.12.0"); err != nil {
		t.Fatalf("addPackageToProject failed: %v", err)
	}
	manifest := readJSONDocument(t, manifestPath)
	var dependencies map[string]string
	if err := json.Unmarshal(manifest["dependencies"], &dependencies); err != nil {
		t.Fatal(err)
	}
	if got := dependencies[source.packageID]; got != source.manifestURL+"#v0.12.0" {
		t.Fatalf("package reference = %q", got)
	}
	if got := mustReadFile(t, lockPath); string(got) != string(lock) {
		t.Fatalf("packages-lock.json changed: got %q, want %q", got, lock)
	}
	assertFileMode(t, manifestPath, 0o600)
	assertFileMode(t, lockPath, 0o640)
}

func TestConfirmPackageAdd(t *testing.T) {
	source := packageSource{
		packageID:   "dev.crysta.uniforge",
		manifestURL: "https://github.com/neptaco/uniforge-unity.git?path=Packages/dev.crysta.uniforge",
	}

	setInteractive := func(t *testing.T, interactive bool) {
		t.Helper()
		original := packageAddIsInteractive
		packageAddIsInteractive = func(*cobra.Command) bool { return interactive }
		t.Cleanup(func() { packageAddIsInteractive = original })
	}
	setYes := func(t *testing.T, yes bool) {
		t.Helper()
		original := packageAddYes
		packageAddYes = yes
		t.Cleanup(func() { packageAddYes = original })
	}

	t.Run("shows resolved values and accepts the default", func(t *testing.T) {
		setInteractive(t, true)
		setYes(t, false)
		var output bytes.Buffer
		command := &cobra.Command{}
		command.SetIn(strings.NewReader("\n"))
		command.SetOut(&output)

		confirmed, err := confirmPackageAdd(command, ".", source, "v0.12.0")
		if err != nil {
			t.Fatal(err)
		}
		if !confirmed {
			t.Fatal("default response did not confirm")
		}
		for _, value := range []string{
			"Package: dev.crysta.uniforge",
			"Source: " + source.manifestURL,
			"Tag: v0.12.0",
			"Reference: " + source.manifestURL + "#v0.12.0",
			filepath.Join("Packages", "manifest.json"),
			"Add this package? [Y/n]:",
		} {
			if !strings.Contains(output.String(), value) {
				t.Fatalf("output does not contain %q:\n%s", value, output.String())
			}
		}
	})

	t.Run("cancels on no", func(t *testing.T) {
		setInteractive(t, true)
		setYes(t, false)
		command := &cobra.Command{}
		command.SetIn(strings.NewReader("no\n"))
		command.SetOut(&bytes.Buffer{})

		confirmed, err := confirmPackageAdd(command, ".", source, "v0.12.0")
		if err != nil {
			t.Fatal(err)
		}
		if confirmed {
			t.Fatal("no response confirmed the package")
		}
	})

	t.Run("non-interactive execution skips the prompt", func(t *testing.T) {
		setInteractive(t, false)
		setYes(t, false)
		var output bytes.Buffer
		command := &cobra.Command{}
		command.SetOut(&output)

		confirmed, err := confirmPackageAdd(command, ".", source, "v0.12.0")
		if err != nil {
			t.Fatal(err)
		}
		if !confirmed || output.Len() != 0 {
			t.Fatalf("confirmed = %v, output = %q", confirmed, output.String())
		}
	})

	t.Run("yes flag skips an interactive prompt", func(t *testing.T) {
		setInteractive(t, true)
		setYes(t, true)
		var output bytes.Buffer
		command := &cobra.Command{}
		command.SetOut(&output)

		confirmed, err := confirmPackageAdd(command, ".", source, "v0.12.0")
		if err != nil {
			t.Fatal(err)
		}
		if !confirmed || output.Len() != 0 {
			t.Fatalf("confirmed = %v, output = %q", confirmed, output.String())
		}
	})
}

func TestPackageAddCommandSurface(t *testing.T) {
	if packageAddCmd.Parent() != packageCmd {
		t.Fatalf("package add parent = %v, want package command", packageAddCmd.Parent())
	}
	if packageAddCmd.Use != "add [project] <package-source>" {
		t.Fatalf("package add use = %q", packageAddCmd.Use)
	}
	if packageAddCmd.Flags().Lookup("tag") == nil {
		t.Fatal("package add command is missing tag flag")
	}
	if packageAddCmd.Flags().Lookup("yes") == nil {
		t.Fatal("package add command is missing yes flag")
	}
	if packageAddCmd.Flags().Lookup("version") != nil {
		t.Fatal("package add command still exposes the old version flag")
	}
}

func TestPackageAddArguments(t *testing.T) {
	t.Run("uses the current project when omitted", func(t *testing.T) {
		project, source := packageAddArguments([]string{"owner/repository/Packages/example"})
		if project != "" || source != "owner/repository/Packages/example" {
			t.Fatalf("project = %q, source = %q", project, source)
		}
	})

	t.Run("accepts an explicit project", func(t *testing.T) {
		project, source := packageAddArguments([]string{"/path/to/project", "owner/repository/Packages/example"})
		if project != "/path/to/project" || source != "owner/repository/Packages/example" {
			t.Fatalf("project = %q, source = %q", project, source)
		}
	})
}
