package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CwdHints struct {
	ProjectPath string
	GitRoot     string
}

func ResolveFromCwd(startDir string) CwdHints {
	dir := startDir
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return CwdHints{}
		}
		dir = cwd
	}

	return CwdHints{
		ProjectPath: findUnityProjectRoot(dir),
		GitRoot:     findGitRoot(dir),
	}
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

func findExplicitProject(explicit string, projects []ProjectInfo) (*ProjectInfo, error) {
	normalizedExplicit := normalizePath(explicit)

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
	current := filepath.Clean(startPath)

	for {
		assetsDir := filepath.Join(current, "Assets")
		projectSettingsDir := filepath.Join(current, "ProjectSettings")
		if isDir(assetsDir) && isDir(projectSettingsDir) {
			return normalizePath(current)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func findGitRoot(startPath string) string {
	current := filepath.Clean(startPath)

	for {
		gitPath := filepath.Join(current, ".git")
		if exists(gitPath) {
			return normalizePath(current)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return ""
		}
		current = parent
	}
}

func normalizePath(pathValue string) string {
	if pathValue == "" {
		return ""
	}

	absolutePath, err := filepath.Abs(pathValue)
	if err != nil {
		absolutePath = pathValue
	}

	normalized := filepath.ToSlash(absolutePath)
	return strings.TrimRight(normalized, "/")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
