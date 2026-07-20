package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type CwdHints struct {
	ProjectPath string
	GitRoot     string
}

func ResolveFromCwd(startDir string) CwdHints {
	dir := resolveStartPath(startDir)
	if dir == "" {
		return CwdHints{}
	}

	return CwdHints{
		ProjectPath: findUnityProjectRoot(dir),
		GitRoot:     findGitRoot(dir),
	}
}

// ResolveUnityProjectPath finds the Unity project containing an existing path
// without scanning for a Git root.
func ResolveUnityProjectPath(startPath string) string {
	return findUnityProjectRoot(resolveStartPath(startPath))
}

func MatchProject(hints CwdHints, projects []ProjectInfo) *ProjectInfo {
	if hints.ProjectPath != "" {
		normalizedHintPath := normalizePath(hints.ProjectPath)
		for index := range projects {
			if normalizePath(projects[index].ID) == normalizedHintPath {
				return &projects[index]
			}
		}
	}

	if hints.GitRoot != "" {
		normalizedGitRoot := normalizePath(hints.GitRoot)
		var matches []*ProjectInfo
		for index := range projects {
			if normalizePath(projects[index].GitRoot) == normalizedGitRoot {
				matches = append(matches, &projects[index])
			}
		}
		if len(matches) == 1 {
			return matches[0]
		}
	}

	return nil
}

func ResolveProject(explicit string, hints CwdHints, projects []ProjectInfo) (*ProjectInfo, error) {
	if explicit != "" {
		project, err := findExplicitProject(explicit, projects)
		if err != nil {
			return nil, err
		}
		if project != nil {
			return project, nil
		}
		return nil, fmt.Errorf("project not found: %s", explicit)
	}

	if matched := MatchProject(hints, projects); matched != nil {
		return matched, nil
	}

	if len(projects) == 1 {
		return &projects[0], nil
	}

	if len(projects) == 0 {
		return nil, fmt.Errorf("no connected Unity projects")
	}

	return nil, fmt.Errorf("multiple Unity projects are connected; use --project")
}

// ResolveProjectOrPath resolves a connected project or a local Unity project
// path. An explicit argument never falls through to cwd or connection-count
// selection when it cannot be resolved.
func ResolveProjectOrPath(explicit, cwd string, projects []ProjectInfo) (*ProjectInfo, string, error) {
	if explicit != "" {
		project, err := findExplicitProjectFromCwd(explicit, cwd, projects)
		if err != nil {
			return nil, "", err
		}
		if project != nil {
			return project, "", nil
		}

		explicitPath := explicit
		if !filepath.IsAbs(explicitPath) && cwd != "" {
			explicitPath = filepath.Join(cwd, explicitPath)
		}
		if projectPath := ResolveUnityProjectPath(explicitPath); projectPath != "" {
			if matched := MatchProject(CwdHints{ProjectPath: projectPath}, projects); matched != nil {
				return matched, "", nil
			}
			return nil, projectPath, nil
		}

		return nil, "", fmt.Errorf("project not found: %s", explicit)
	}

	startPath := resolveStartPath(cwd)
	if projectPath := findUnityProjectRoot(startPath); projectPath != "" {
		if matched := MatchProject(CwdHints{ProjectPath: projectPath}, projects); matched != nil {
			return matched, "", nil
		}
		return nil, projectPath, nil
	}

	if gitRoot := findGitRoot(startPath); gitRoot != "" {
		if matched := MatchProject(CwdHints{GitRoot: gitRoot}, projects); matched != nil {
			return matched, "", nil
		}
	}

	if len(projects) == 1 {
		return &projects[0], "", nil
	}
	if len(projects) > 1 {
		return nil, "", fmt.Errorf("multiple Unity projects are connected; specify a project argument")
	}
	return nil, "", nil
}

func findExplicitProject(explicit string, projects []ProjectInfo) (*ProjectInfo, error) {
	return findExplicitProjectFromCwd(explicit, "", projects)
}

func findExplicitProjectFromCwd(explicit, cwd string, projects []ProjectInfo) (*ProjectInfo, error) {
	explicitID := explicit
	if !filepath.IsAbs(explicitID) && cwd != "" {
		explicitID = filepath.Join(cwd, explicitID)
	}
	normalizedExplicit := normalizePath(explicitID)

	for index := range projects {
		project := &projects[index]
		if project.ID == explicit || normalizePath(project.ID) == normalizedExplicit {
			return project, nil
		}
	}

	var exactNameMatches []*ProjectInfo
	for index := range projects {
		project := &projects[index]
		if strings.EqualFold(project.Name, explicit) {
			exactNameMatches = append(exactNameMatches, project)
		}
	}
	if len(exactNameMatches) == 1 {
		return exactNameMatches[0], nil
	}
	if len(exactNameMatches) > 1 {
		return nil, fmt.Errorf("multiple projects match name %q", explicit)
	}

	var partialMatches []*ProjectInfo
	lowerExplicit := strings.ToLower(explicit)
	for index := range projects {
		project := &projects[index]
		if strings.Contains(strings.ToLower(project.Name), lowerExplicit) {
			partialMatches = append(partialMatches, project)
		}
	}
	if len(partialMatches) == 1 {
		return partialMatches[0], nil
	}
	if len(partialMatches) > 1 {
		return nil, fmt.Errorf("multiple projects partially match %q", explicit)
	}

	return nil, nil
}

func findUnityProjectRoot(startPath string) string {
	current := existingStartDirectory(startPath)
	if current == "" {
		return ""
	}

	for {
		assetsDir := filepath.Join(current, "Assets")
		projectSettingsDir := filepath.Join(current, "ProjectSettings")
		if isDir(assetsDir) && isDir(projectSettingsDir) {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func findGitRoot(startPath string) string {
	current := existingStartDirectory(startPath)
	if current == "" {
		return ""
	}

	for {
		gitPath := filepath.Join(current, ".git")
		if exists(gitPath) {
			return current
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func resolveStartPath(startPath string) string {
	if startPath == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return ""
		}
		startPath = cwd
	}

	absolutePath, err := filepath.Abs(startPath)
	if err != nil {
		return ""
	}
	return filepath.Clean(absolutePath)
}

func existingStartDirectory(startPath string) string {
	if startPath == "" {
		return ""
	}
	info, err := os.Stat(startPath)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		return filepath.Dir(startPath)
	}
	return startPath
}

func normalizePath(pathValue string) string {
	if pathValue == "" {
		return ""
	}

	absolutePath, err := filepath.Abs(pathValue)
	if err != nil {
		absolutePath = pathValue
	}

	normalized := filepath.ToSlash(filepath.Clean(absolutePath))
	if runtime.GOOS == "windows" {
		normalized = strings.ToLower(normalized)
	}
	return normalized
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
