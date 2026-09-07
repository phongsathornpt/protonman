package modelprofile

import "reflect"

// PublishInputSchema adapts a canonical Proton tool schema to the subset a
// model family can reliably consume. Runtime validation still uses the
// canonical schema, so lowering never weakens host-side argument checks.
func PublishInputSchema(profile Resolved, schema map[string]any) map[string]any {
	if profile.Compatibility.ToolSchemaDialect != ToolSchemaGeminiSubset {
		return cloneSchemaValue(schema).(map[string]any)
	}
	return lowerGeminiSchema(schema)
}

func lowerGeminiSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return map[string]any{}
	}
	if raw, ok := schema["oneOf"]; ok {
		if branches, ok := toAnySlice(raw); ok {
			if merged, ok := mergeObjectBranches(branches); ok {
				return lowerGeminiSchema(merged)
			}
		}
	}
	out := make(map[string]any)
	for _, key := range []string{"type", "description", "enum"} {
		if value, ok := schema[key]; ok {
			out[key] = cloneSchemaValue(value)
		}
	}
	if constant, ok := schema["const"]; ok {
		out["enum"] = []any{cloneSchemaValue(constant)}
		if _, exists := out["type"]; !exists {
			out["type"] = inferSchemaType(constant)
		}
	}
	if raw, ok := schema["properties"].(map[string]any); ok {
		properties := make(map[string]any, len(raw))
		for name, value := range raw {
			if child, ok := value.(map[string]any); ok {
				properties[name] = lowerGeminiSchema(child)
			}
		}
		out["properties"] = properties
	}
	if required, ok := toAnySlice(schema["required"]); ok && len(required) > 0 {
		out["required"] = cloneSchemaValue(required)
	}
	if child, ok := schema["items"].(map[string]any); ok {
		out["items"] = lowerGeminiSchema(child)
	}
	return out
}

func mergeObjectBranches(branches []any) (map[string]any, bool) {
	if len(branches) == 0 {
		return nil, false
	}
	mergedProps := map[string]any{}
	var required map[string]struct{}
	for index, raw := range branches {
		branch, ok := raw.(map[string]any)
		if !ok || branch["type"] != "object" {
			return nil, false
		}
		props, _ := branch["properties"].(map[string]any)
		for name, rawProp := range props {
			prop, ok := rawProp.(map[string]any)
			if !ok {
				continue
			}
			if existing, exists := mergedProps[name].(map[string]any); exists {
				mergedProps[name] = mergePropertySchemas(existing, prop)
			} else {
				mergedProps[name] = cloneSchemaValue(prop)
			}
		}
		branchRequired := stringSet(branch["required"])
		if index == 0 {
			required = branchRequired
		} else {
			for name := range required {
				if _, ok := branchRequired[name]; !ok {
					delete(required, name)
				}
			}
		}
	}
	out := map[string]any{"type": "object", "properties": mergedProps}
	if len(required) > 0 {
		items := make([]any, 0, len(required))
		for name := range required {
			items = append(items, name)
		}
		out["required"] = items
	}
	return out, true
}

func mergePropertySchemas(a, b map[string]any) map[string]any {
	if reflect.DeepEqual(a, b) {
		return cloneSchemaValue(a).(map[string]any)
	}
	out := cloneSchemaValue(a).(map[string]any)
	values := enumValues(a)
	values = appendUnique(values, enumValues(b)...)
	if len(values) > 0 {
		delete(out, "const")
		out["enum"] = values
		if _, ok := out["type"]; !ok {
			out["type"] = inferSchemaType(values[0])
		}
	}
	return out
}

func enumValues(schema map[string]any) []any {
	if value, ok := schema["const"]; ok {
		return []any{cloneSchemaValue(value)}
	}
	values, _ := toAnySlice(schema["enum"])
	return append([]any(nil), values...)
}

func appendUnique(dst []any, values ...any) []any {
	for _, value := range values {
		found := false
		for _, existing := range dst {
			if reflect.DeepEqual(existing, value) {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, cloneSchemaValue(value))
		}
	}
	return dst
}

func stringSet(raw any) map[string]struct{} {
	values, _ := toAnySlice(raw)
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out[text] = struct{}{}
		}
	}
	return out
}

func toAnySlice(raw any) ([]any, bool) {
	switch values := raw.(type) {
	case []any:
		return values, true
	case []string:
		out := make([]any, len(values))
		for i := range values {
			out[i] = values[i]
		}
		return out, true
	default:
		return nil, false
	}
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = cloneSchemaValue(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = cloneSchemaValue(child)
		}
		return out
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out
	default:
		return typed
	}
}

func inferSchemaType(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case float32, float64:
		return "number"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "integer"
	default:
		return "string"
	}
}
