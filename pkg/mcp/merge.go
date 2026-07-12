package mcp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/neptaco/uniforge/pkg/bridge"
	toolpkg "github.com/neptaco/uniforge/pkg/tools"
)

func StaticToolMetadata() []ToolMetadata {
	definitions := toolpkg.BaseDefinitions()
	metadata := make([]ToolMetadata, 0, len(definitions))
	for _, definition := range definitions {
		metadata = append(metadata, ToolMetadata{
			ToolDefinition: definition,
		})
	}
	return metadata
}

// MergeDynamicToolDefinitions merges dynamic bridge tool registrations by name.
func MergeDynamicToolDefinitions(projects []bridge.ProjectInfo) []ToolMetadata {
	merged := toolpkg.MergeDynamicDefinitions(projects, toolpkg.BaseToolNames(), len(projects) > 1)
	result := make([]ToolMetadata, 0, len(merged))
	for _, entry := range merged {
		metadata := ToolMetadata{
			ToolDefinition: entry.Tool,
			HasConflicts:   entry.HasConflicts,
			Sources:        make([]ToolSource, 0, len(entry.ProjectIDs)),
		}
		for index := range entry.ProjectIDs {
			source := ToolSource{ID: entry.ProjectIDs[index]}
			if index < len(entry.ProjectNames) {
				source.Name = entry.ProjectNames[index]
			}
			metadata.Sources = append(metadata.Sources, source)
		}
		sort.Slice(metadata.Sources, func(i, j int) bool {
			if strings.EqualFold(metadata.Sources[i].Name, metadata.Sources[j].Name) {
				return metadata.Sources[i].ID < metadata.Sources[j].ID
			}
			return strings.ToLower(metadata.Sources[i].Name) < strings.ToLower(metadata.Sources[j].Name)
		})
		result = append(result, metadata)
	}
	return result
}

// FindToolMetadata resolves a single merged tool by name.
func FindToolMetadata(tools []ToolMetadata, name string) (*ToolMetadata, error) {
	for index := range tools {
		if tools[index].Name == name {
			return &tools[index], nil
		}
	}

	var folded []*ToolMetadata
	for index := range tools {
		if strings.EqualFold(tools[index].Name, name) {
			folded = append(folded, &tools[index])
		}
	}

	switch len(folded) {
	case 0:
		return nil, fmt.Errorf("tool not found: %s", name)
	case 1:
		return folded[0], nil
	default:
		return nil, fmt.Errorf("multiple tools match %q", name)
	}
}
