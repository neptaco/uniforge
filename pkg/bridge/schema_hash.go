package bridge

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// ComputeSchemaHash computes a stable hash over tool calling interfaces (name + inputSchema).
// The hash changes when tools are added/removed or their input schemas change.
func ComputeSchemaHash(tools []ToolDefinition) string {
	type entry struct {
		Name        string         `json:"name"`
		InputSchema map[string]any `json:"inputSchema"`
	}

	entries := make([]entry, len(tools))
	for i, t := range tools {
		entries[i] = entry{Name: t.Name, InputSchema: t.InputSchema}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	data, _ := json.Marshal(entries)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:4])
}
