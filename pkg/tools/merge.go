package tools

import (
	"reflect"
	"sort"

	"github.com/neptaco/uniforge/pkg/bridge"
)

type toolOccurrence struct {
	project bridge.ProjectInfo
	tool    bridge.ToolDefinition
}

func MergeDynamicDefinitions(projects []bridge.ProjectInfo, excludeNames map[string]struct{}, includeProjectSelector bool) []MergedDefinition {
	occurrencesByName := map[string][]toolOccurrence{}
	for _, project := range projects {
		for _, tool := range project.Tools {
			if _, excluded := excludeNames[tool.Name]; excluded {
				continue
			}
			occurrencesByName[tool.Name] = append(occurrencesByName[tool.Name], toolOccurrence{
				project: project,
				tool:    tool,
			})
		}
	}

	names := make([]string, 0, len(occurrencesByName))
	for name := range occurrencesByName {
		names = append(names, name)
	}
	sort.Strings(names)

	merged := make([]MergedDefinition, 0, len(names))
	for _, name := range names {
		occurrences := occurrencesByName[name]
		first := occurrences[0]
		tool := cloneToolDefinition(first.tool)
		duplicated := len(occurrences) > 1
		hasConflicts := false

		for _, occurrence := range occurrences[1:] {
			if !toolDefinitionsEqual(first.tool, occurrence.tool) {
				hasConflicts = true
				break
			}
		}

		if duplicated {
			tool.InputSchema = mergeInputSchemas(occurrences)
			tool.Description = first.tool.Description + "\n\nMultiple connected Unity projects provide this tool. UniForge will try to resolve the project from the current working directory before requiring project_id."
		} else {
			tool.InputSchema = normalizeInputSchema(first.tool.InputSchema)
		}

		if includeProjectSelector {
			properties, _ := tool.InputSchema["properties"].(map[string]any)
			if properties == nil {
				properties = map[string]any{}
			}
			properties["project_id"] = buildProjectIDProperty(occurrences, duplicated)
			tool.InputSchema["properties"] = properties
		}

		entry := MergedDefinition{
			Tool:         cloneToolDefinition(tool),
			HasConflicts: hasConflicts,
		}
		for _, occurrence := range occurrences {
			entry.ProjectIDs = append(entry.ProjectIDs, occurrence.project.ID)
			entry.ProjectNames = append(entry.ProjectNames, occurrence.project.Name)
		}
		merged = append(merged, entry)
	}

	return merged
}

func normalizeInputSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return schemaObject(nil, nil)
	}

	properties, _ := schema["properties"].(map[string]any)
	required := requiredNames(schema["required"])
	return schemaObject(cloneMap(properties), required)
}

func mergeInputSchemas(occurrences []toolOccurrence) map[string]any {
	if len(occurrences) == 0 {
		return schemaObject(nil, nil)
	}

	mergedProperties := map[string]any{}
	var required []string
	for index, occurrence := range occurrences {
		normalized := normalizeInputSchema(occurrence.tool.InputSchema)
		properties, _ := normalized["properties"].(map[string]any)
		for key, value := range properties {
			mergedProperties[key] = value
		}

		currentRequired := requiredNames(normalized["required"])
		if index == 0 {
			required = currentRequired
			continue
		}

		currentRequiredSet := map[string]struct{}{}
		for _, name := range currentRequired {
			currentRequiredSet[name] = struct{}{}
		}

		intersection := make([]string, 0, len(required))
		for _, name := range required {
			if _, ok := currentRequiredSet[name]; ok {
				intersection = append(intersection, name)
			}
		}
		required = intersection
	}

	return schemaObject(mergedProperties, required)
}

func buildProjectIDProperty(occurrences []toolOccurrence, duplicated bool) map[string]any {
	description := "Optional target Unity project ID override."
	if duplicated {
		description = "Target Unity project ID. UniForge first tries current working directory matching; if ambiguous, specify this explicitly."
	}

	enumValues := make([]any, 0, len(occurrences))
	for _, occurrence := range occurrences {
		enumValues = append(enumValues, occurrence.project.ID)
	}

	property := map[string]any{
		"type":        "string",
		"description": description,
	}
	if len(enumValues) > 0 {
		property["enum"] = enumValues
	}
	return property
}

func cloneToolDefinition(tool bridge.ToolDefinition) bridge.ToolDefinition {
	clone := tool
	if tool.InputSchema != nil {
		clone.InputSchema = cloneMap(tool.InputSchema)
	}
	if tool.OutputSchema != nil {
		clone.OutputSchema = cloneMap(tool.OutputSchema)
	}
	if tool.Annotations != nil {
		clone.Annotations = cloneMap(tool.Annotations)
	}
	return clone
}

func toolDefinitionsEqual(a, b bridge.ToolDefinition) bool {
	return a.Name == b.Name &&
		a.Description == b.Description &&
		reflect.DeepEqual(a.InputSchema, b.InputSchema) &&
		reflect.DeepEqual(a.OutputSchema, b.OutputSchema) &&
		reflect.DeepEqual(a.Annotations, b.Annotations)
}
