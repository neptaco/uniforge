package tools

import "testing"

func TestBuildExamplePayloadUsesRequiredFieldsAndDefaults(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"required_name": map[string]any{"type": "string"},
			"optional_flag": map[string]any{"type": "boolean", "default": true},
		},
		"required": []any{"required_name"},
	}

	payload, ok := BuildExamplePayload(schema).(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map[string]any", BuildExamplePayload(schema))
	}
	if len(payload) != 1 {
		t.Fatalf("len(payload) = %d, want 1", len(payload))
	}
	if got := payload["required_name"]; got != "<string>" {
		t.Fatalf("payload[required_name] = %#v, want %q", got, "<string>")
	}
}

func TestBuildExamplePayloadSupportsAdditionalProperties(t *testing.T) {
	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": true,
	}

	payload, ok := BuildExamplePayload(schema).(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map[string]any", BuildExamplePayload(schema))
	}
	if got := payload["<key>"]; got != "<value>" {
		t.Fatalf("payload[<key>] = %#v, want %q", got, "<value>")
	}
}

func TestNormalizeSchemaCollapsesDictionaryPattern(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"comparer": map[string]any{"type": "object"},
			"count":    map[string]any{"type": "integer"},
			"item":     map[string]any{"type": "object"},
			"keys":     map[string]any{"type": "object"},
			"values":   map[string]any{"type": "object"},
		},
	}

	normalized := NormalizeSchema(schema)
	if _, ok := normalized["properties"]; ok {
		t.Fatalf("normalized schema still has properties")
	}
	if got, ok := normalized["additionalProperties"].(bool); !ok || !got {
		t.Fatalf("additionalProperties = %#v, want true", normalized["additionalProperties"])
	}
}
