package tools

import (
	"fmt"
)

func Execute(deps ExecutionDeps, name string, args map[string]any) (any, error) {
	switch name {
	case "list-projects":
		return executeListProjects(deps)
	default:
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
}

func executeListProjects(deps ExecutionDeps) (any, error) {
	if deps.Client == nil {
		return nil, fmt.Errorf("daemon client is required")
	}

	result, err := deps.Client.ListProjects(false)
	if err != nil {
		return nil, err
	}

	projects := make([]map[string]any, 0, len(result.Projects))
	for _, project := range result.Projects {
		projects = append(projects, map[string]any{
			"id":   project.ID,
			"name": project.Name,
		})
	}

	return map[string]any{"projects": projects}, nil
}
