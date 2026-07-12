package tools

import "github.com/neptaco/uniforge/pkg/bridge"

var baseDefinitions = []bridge.ToolDefinition{
	{
		Name:        "list-projects",
		Description: "List all connected Unity projects",
		InputSchema: schemaObject(nil, nil),
		Annotations: queryAnnotations("List Unity Projects"),
	},
}

func BaseDefinitions() []bridge.ToolDefinition {
	definitions := make([]bridge.ToolDefinition, len(baseDefinitions))
	copy(definitions, baseDefinitions)
	return definitions
}

func BaseToolNames() map[string]struct{} {
	names := make(map[string]struct{}, len(baseDefinitions))
	for _, definition := range baseDefinitions {
		names[definition.Name] = struct{}{}
	}
	return names
}

func FindBaseDefinition(name string) (*bridge.ToolDefinition, bool) {
	for _, definition := range baseDefinitions {
		if definition.Name != name {
			continue
		}

		copyDefinition := definition
		return &copyDefinition, true
	}

	return nil, false
}

func IsBaseTool(name string) bool {
	_, ok := FindBaseDefinition(name)
	return ok
}

func schemaObject(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	if properties != nil {
		schema["properties"] = properties
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func queryAnnotations(title string) map[string]any {
	return map[string]any{
		"title":           title,
		"readOnlyHint":    true,
		"destructiveHint": false,
		"idempotentHint":  true,
		"openWorldHint":   false,
	}
}
