package cmd

import (
	"errors"
	"fmt"

	"github.com/neptaco/uniforge/pkg/hub"
	"github.com/neptaco/uniforge/pkg/ui"
	"github.com/neptaco/uniforge/pkg/unity"
)

func resolveLoadedProjectArg(args []string) (*unity.Project, error) {
	if len(args) > 0 {
		return resolveLoadedProject(args[0], true)
	}
	return resolveLoadedProject(".", false)
}

func resolveLoadedProject(candidate string, explicit bool) (*unity.Project, error) {
	project, err := unity.LoadProject(candidate)
	if err == nil {
		return project, nil
	}

	if explicit {
		hubProject, hubErr := findHubProject(candidate)
		if hubErr != nil {
			var multiErr *hub.MultipleMatchError
			if errors.As(hubErr, &multiErr) {
				return nil, hubErr
			}
			return nil, fmt.Errorf("failed to load project: %w", err)
		}
		return unity.LoadProject(hubProject.Path)
	}

	hubProject, hubErr := selectAnyHubProject()
	if hubErr != nil {
		return nil, fmt.Errorf("failed to load project: %w", err)
	}
	return unity.LoadProject(hubProject.Path)
}

func resolveHubProjectArg(args []string) (*hub.ProjectInfo, error) {
	if len(args) > 0 {
		return findHubProject(args[0])
	}
	return selectAnyHubProject()
}

func findHubProject(query string) (*hub.ProjectInfo, error) {
	hubClient := hub.NewClient()
	hubProject, err := hubClient.GetProject(query)
	if err == nil {
		return hubProject, nil
	}

	var multiErr *hub.MultipleMatchError
	if errors.As(err, &multiErr) {
		return selectProject(multiErr.Matches, query)
	}

	return nil, err
}

func selectAnyHubProject() (*hub.ProjectInfo, error) {
	hubClient := hub.NewClient()
	projects, err := hubClient.ListProjects()
	if err != nil {
		return nil, err
	}
	if len(projects) == 0 {
		return nil, fmt.Errorf("no projects registered in Unity Hub")
	}
	return selectProject(projects, "")
}

func selectProject(matches []hub.ProjectInfo, query string) (*hub.ProjectInfo, error) {
	if len(matches) == 0 {
		if query == "" {
			return nil, fmt.Errorf("no projects registered in Unity Hub")
		}
		return nil, fmt.Errorf("no projects match %q", query)
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}

	if !ui.IsTTY() {
		if query == "" {
			return nil, fmt.Errorf("project argument is required in non-interactive mode")
		}
		ui.Error("Multiple projects match '%s':", query)
		for _, p := range matches {
			ui.Print("  - %s (%s)", p.Title, p.Version)
		}
		return nil, fmt.Errorf("multiple projects match '%s', please be more specific", query)
	}

	options := make([]ui.SelectOption, len(matches))
	for i, p := range matches {
		options[i] = ui.SelectOption{
			Label:       p.Title,
			Description: p.Version,
			Value:       i,
		}
	}

	prompt := "Select a Unity project:"
	if query != "" {
		prompt = fmt.Sprintf("Multiple projects match '%s':", query)
	}

	selected := ui.Select(prompt, options)
	if selected < 0 {
		return nil, fmt.Errorf("cancelled")
	}

	return &matches[selected], nil
}
