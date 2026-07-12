package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/spf13/cobra"
)

var (
	projectInfoFormat string
)

var projectInfoCmd = &cobra.Command{
	Use:   "info [project]",
	Short: "Show project information",
	Long: `Display detailed information about a Unity project.

Shows Unity version, installed packages, and assembly definitions.

Examples:
  # Show info for current directory
  uniforge project info

  # Show info for specific project
  uniforge project info /path/to/project

  # JSON output
  uniforge project info --output json`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProjectInfo,
}

func init() {
	projectCmd.AddCommand(projectInfoCmd)

	addOutputFlag(projectInfoCmd, &projectInfoFormat, "Output format: text, json")
}

type projectInfoOutput struct {
	Name       string            `json:"name"`
	Path       string            `json:"path"`
	Unity      string            `json:"unity_version"`
	Changeset  string            `json:"changeset,omitempty"`
	Packages   map[string]string `json:"packages,omitempty"`
	Assemblies []asmdefEntry     `json:"assemblies,omitempty"`
}

type asmdefEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

func runProjectInfo(cmd *cobra.Command, args []string) error {
	project, err := resolveLoadedProjectArg(args)
	if err != nil {
		return err
	}

	packages, _ := loadManifestPackages(project.Path)
	assemblies, _ := findAssemblyDefinitions(project.Path)

	info := projectInfoOutput{
		Name:       project.Name,
		Path:       project.Path,
		Unity:      project.UnityVersion,
		Changeset:  project.Changeset,
		Packages:   packages,
		Assemblies: assemblies,
	}

	if resolveOutputOrDefault(projectInfoFormat, "text") == "json" {
		return printProjectInfoJSON(info)
	}
	return printProjectInfoText(info)
}

func printProjectInfoJSON(info projectInfoOutput) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(info)
}

func printProjectInfoText(info projectInfoOutput) error {
	ui.Print("Project: %s", info.Name)
	ui.Print("Path:    %s", info.Path)
	ui.Print("Unity:   %s", info.Unity)
	if info.Changeset != "" {
		ui.Print("Changeset: %s", info.Changeset)
	}

	if len(info.Packages) > 0 {
		ui.Print("")
		ui.Print("Packages (%d):", len(info.Packages))

		// Sort package names for consistent output
		names := make([]string, 0, len(info.Packages))
		for name := range info.Packages {
			names = append(names, name)
		}
		sort.Strings(names)

		maxNameLen := 0
		for _, name := range names {
			if len(name) > maxNameLen {
				maxNameLen = len(name)
			}
		}

		for _, name := range names {
			ui.Print("  %-*s  %s", maxNameLen, name, info.Packages[name])
		}
	}

	if len(info.Assemblies) > 0 {
		ui.Print("")
		ui.Print("Assembly Definitions (%d):", len(info.Assemblies))

		maxNameLen := 0
		for _, asm := range info.Assemblies {
			if len(asm.Name) > maxNameLen {
				maxNameLen = len(asm.Name)
			}
		}

		for _, asm := range info.Assemblies {
			ui.Print("  %-*s  %s", maxNameLen, asm.Name, asm.Path)
		}
	}

	return nil
}

// loadManifestPackages reads Packages/manifest.json and returns the dependencies map.
func loadManifestPackages(projectPath string) (map[string]string, error) {
	manifestPath := filepath.Join(projectPath, "Packages", "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return manifest.Dependencies, nil
}

// findAssemblyDefinitions finds all .asmdef files under Assets/ directory.
func findAssemblyDefinitions(projectPath string) ([]asmdefEntry, error) {
	assetsDir := filepath.Join(projectPath, "Assets")
	var entries []asmdefEntry

	err := filepath.Walk(assetsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible directories
		}
		if !info.IsDir() && strings.HasSuffix(path, ".asmdef") {
			relPath, _ := filepath.Rel(projectPath, path)

			// Read the asmdef to get the name field
			name := readAsmdefName(path)
			if name == "" {
				name = strings.TrimSuffix(info.Name(), ".asmdef")
			}

			entries = append(entries, asmdefEntry{
				Name: name,
				Path: relPath,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// readAsmdefName reads the "name" field from an asmdef JSON file.
func readAsmdefName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var asmdef struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &asmdef); err != nil {
		return ""
	}
	return asmdef.Name
}
