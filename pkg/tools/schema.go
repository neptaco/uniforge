package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func NormalizeSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{}
	}
	return normalizeSchemaNode(schema).(map[string]any)
}

func BuildExamplePayload(schema map[string]any) any {
	return buildExampleValue(NormalizeSchema(schema), 0)
}

func normalizeSchemaNode(node any) any {
	switch typed := node.(type) {
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, item := range typed {
			normalized = append(normalized, normalizeSchemaNode(item))
		}
		return normalized
	case map[string]any:
		normalized := map[string]any{}
		for key, value := range typed {
			switch key {
			case "properties":
				properties, _ := value.(map[string]any)
				normalizedProperties := map[string]any{}
				for propertyName, propertyValue := range properties {
					normalizedProperties[propertyName] = normalizeSchemaNode(propertyValue)
				}
				normalized[key] = normalizedProperties
			case "items":
				normalized[key] = normalizeSchemaNode(value)
			default:
				normalized[key] = value
			}
		}
		if looksLikeDictionarySchema(normalized) {
			result := cloneMap(normalized)
			delete(result, "properties")
			result["additionalProperties"] = true
			if description, _ := result["description"].(string); description == "" {
				result["description"] = "Arbitrary key/value object"
			}
			return result
		}
		return normalized
	default:
		return node
	}
}

func buildExampleValue(schema map[string]any, depth int) any {
	if depth > 4 {
		return nil
	}

	if schema == nil {
		return map[string]any{}
	}

	switch schema["type"] {
	case "object":
		if additionalProperties, ok := schema["additionalProperties"]; ok {
			switch typed := additionalProperties.(type) {
			case bool:
				if typed {
					return map[string]any{"<key>": "<value>"}
				}
			case map[string]any:
				return map[string]any{"<key>": buildExampleValue(typed, depth+1)}
			}
		}

		properties, _ := schema["properties"].(map[string]any)
		if len(properties) == 0 {
			return map[string]any{}
		}

		requiredNames := requiredNames(schema["required"])
		sort.Strings(requiredNames)
		example := map[string]any{}
		for _, name := range requiredNames {
			propertySchema, _ := properties[name].(map[string]any)
			if propertySchema == nil {
				continue
			}
			example[name] = buildExampleValue(propertySchema, depth+1)
		}

		names := sortedMapKeys(properties)
		if len(example) > 0 {
			return example
		}

		for _, name := range names {
			propertySchema, _ := properties[name].(map[string]any)
			if propertySchema == nil {
				continue
			}
			example[name] = buildExampleValue(propertySchema, depth+1)
		}
		return example
	case "array":
		switch items := schema["items"].(type) {
		case map[string]any:
			return []any{buildExampleValue(items, depth+1)}
		default:
			return []any{map[string]any{}}
		}
	case "integer":
		if defaultValue, ok := numericDefault(schema["default"]); ok {
			return int(defaultValue)
		}
		return 0
	case "number":
		if defaultValue, ok := numericDefault(schema["default"]); ok {
			return defaultValue
		}
		return 0
	case "boolean":
		if defaultValue, ok := schema["default"].(bool); ok {
			return defaultValue
		}
		return false
	case "string":
		if enumValues, ok := schema["enum"].([]any); ok && len(enumValues) > 0 {
			return fmt.Sprintf("%v", enumValues[0])
		}
		if enumValues, ok := schema["enum"].([]string); ok && len(enumValues) > 0 {
			return enumValues[0]
		}
		if defaultValue, ok := schema["default"].(string); ok {
			return defaultValue
		}
		return "<string>"
	default:
		return map[string]any{}
	}
}

// BuildAnnotatedExample は JSONC 形式のアノテーション付き Example を生成する。
// 各フィールドに description / enum / default / required をコメントとして付与する。
func BuildAnnotatedExample(schema map[string]any) string {
	normalized := NormalizeSchema(schema)
	var sb strings.Builder
	buildAnnotatedObject(&sb, normalized, "", true, 0)
	return strings.TrimRight(sb.String(), "\n")
}

func buildAnnotatedObject(sb *strings.Builder, schema map[string]any, indent string, isRoot bool, depth int) {
	if depth > 4 {
		sb.WriteString("{}")
		return
	}

	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		sb.WriteString("{}")
		return
	}

	reqSet := map[string]bool{}
	for _, r := range requiredNames(schema["required"]) {
		reqSet[r] = true
	}

	sb.WriteString("{\n")
	names := sortedMapKeys(properties)
	childIndent := indent + "  "

	for i, name := range names {
		prop, _ := properties[name].(map[string]any)
		if prop == nil {
			continue
		}

		sb.WriteString(childIndent)
		fmt.Fprintf(sb, "%q: ", name)
		buildAnnotatedValue(sb, prop, childIndent, depth+1)

		if i < len(names)-1 {
			sb.WriteString(",")
		}

		// コメント生成
		comment := buildFieldComment(prop, reqSet[name])
		if comment != "" {
			sb.WriteString("  // ")
			sb.WriteString(comment)
		}
		sb.WriteString("\n")
	}

	sb.WriteString(indent)
	sb.WriteString("}")
}

func buildAnnotatedValue(sb *strings.Builder, schema map[string]any, indent string, depth int) {
	if depth > 5 {
		sb.WriteString("...")
		return
	}

	t, _ := schema["type"].(string)

	switch t {
	case "object":
		if ap, ok := schema["additionalProperties"]; ok {
			if b, ok := ap.(bool); ok && b {
				sb.WriteString("{ \"<key>\": \"<value>\" }")
				return
			}
		}
		buildAnnotatedObject(sb, schema, indent, false, depth)

	case "array":
		items, _ := schema["items"].(map[string]any)
		if items == nil {
			sb.WriteString("[]")
			return
		}
		itemType, _ := items["type"].(string)
		if itemType == "object" {
			_, hasProps := items["properties"].(map[string]any)
			if hasProps {
				sb.WriteString("[\n")
				sb.WriteString(indent + "  ")
				buildAnnotatedObject(sb, items, indent+"  ", false, depth)
				sb.WriteString("\n")
				sb.WriteString(indent)
				sb.WriteString("]")
				return
			}
		}
		// プリミティブ配列
		sb.WriteString("[")
		buildAnnotatedValue(sb, items, indent, depth+1)
		sb.WriteString("]")

	case "string":
		if enumValues := extractEnumValues(schema); len(enumValues) > 0 {
			fmt.Fprintf(sb, "%q", enumValues[0])
		} else if def, ok := schema["default"].(string); ok {
			fmt.Fprintf(sb, "%q", def)
		} else {
			sb.WriteString("\"<string>\"")
		}

	case "integer":
		if def, ok := numericDefault(schema["default"]); ok {
			fmt.Fprintf(sb, "%d", int(def))
		} else {
			sb.WriteString("0")
		}

	case "number":
		if def, ok := numericDefault(schema["default"]); ok {
			fmt.Fprintf(sb, "%g", def)
		} else {
			sb.WriteString("0")
		}

	case "boolean":
		if def, ok := schema["default"].(bool); ok {
			fmt.Fprintf(sb, "%t", def)
		} else {
			sb.WriteString("false")
		}

	default:
		sb.WriteString("{}")
	}
}

func buildFieldComment(schema map[string]any, isRequired bool) string {
	var parts []string

	if isRequired {
		parts = append(parts, "REQUIRED")
	}

	if desc, _ := schema["description"].(string); desc != "" {
		parts = append(parts, desc)
	}

	if enumValues := extractEnumValues(schema); len(enumValues) > 0 {
		parts = append(parts, "enum: "+strings.Join(enumValues, ", "))
	}

	if def, ok := schema["default"]; ok {
		parts = append(parts, fmt.Sprintf("default: %v", def))
	}

	return strings.Join(parts, " | ")
}

func extractEnumValues(schema map[string]any) []string {
	if vals, ok := schema["enum"].([]any); ok {
		result := make([]string, 0, len(vals))
		for _, v := range vals {
			result = append(result, fmt.Sprint(v))
		}
		return result
	}
	if vals, ok := schema["enum"].([]string); ok {
		return vals
	}
	return nil
}

func JSONSchemaString(schema map[string]any) string {
	data, _ := json.MarshalIndent(NormalizeSchema(schema), "", "  ")
	return string(data)
}

func requiredNames(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		names := make([]string, 0, len(typed))
		for _, item := range typed {
			if name, ok := item.(string); ok {
				names = append(names, name)
			}
		}
		return names
	default:
		return nil
	}
}

func looksLikeDictionarySchema(schema map[string]any) bool {
	if schema["type"] != "object" {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	keys := sortedMapKeys(properties)
	expected := []string{"comparer", "count", "item", "keys", "values"}
	if len(keys) != len(expected) {
		return false
	}
	for index, key := range expected {
		if keys[index] != key {
			return false
		}
	}
	return true
}

func numericDefault(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneMap(source map[string]any) map[string]any {
	destination := make(map[string]any, len(source))
	for key, value := range source {
		destination[key] = value
	}
	return destination
}
