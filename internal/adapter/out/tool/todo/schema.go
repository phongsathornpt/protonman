package todotool

func todoUpdateInputSchema() map[string]any {
	operation := func(op string, fields map[string]any, required ...string) map[string]any {
		properties := map[string]any{
			"op": map[string]any{"type": "string", "const": op, "description": "Patch operation kind."},
			"id": map[string]any{"type": "string", "description": "Stable task id from the current snapshot, or a new stable id when op=add."},
		}
		for name, schema := range fields {
			properties[name] = schema
		}
		requiredFields := []any{"op", "id"}
		for _, field := range required {
			requiredFields = append(requiredFields, field)
		}
		return map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             requiredFields,
			"additionalProperties": false,
		}
	}
	text := map[string]any{"type": "string", "description": "Task text as a single safe markdown line."}
	status := map[string]any{"type": "string", "enum": []any{"pending", "in_progress", "completed"}, "description": "Task status."}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expected_revision": map[string]any{"type": "integer", "minimum": 0, "description": "Integer revision from the latest todo action=get snapshot. Send this as a JSON number, never a quoted string; stale revisions are rejected."},
			"operations": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 256,
				"description": "JSON array of patch-operation objects, never a JSON-encoded string. Unmentioned tasks are preserved.",
				"items": map[string]any{"oneOf": []any{
					operation("add", map[string]any{"text": text, "status": status}, "text", "status"),
					operation("set_status", map[string]any{"status": status}, "status"),
					operation("set_text", map[string]any{"text": text}, "text"),
					operation("remove", nil),
				}},
			},
		},
		"required":             []any{"expected_revision", "operations"},
		"additionalProperties": false,
	}
}

func todoItemSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":     map[string]any{"type": "string"},
			"text":   map[string]any{"type": "string"},
			"status": map[string]any{"type": "string", "enum": []any{"pending", "in_progress", "completed"}},
		},
		"required":             []any{"id", "text", "status"},
		"additionalProperties": false,
	}
}

func todoSnapshotSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string"},
			"revision":   map[string]any{"type": "integer", "minimum": 0},
			"items": map[string]any{
				"type":  "array",
				"items": todoItemSchema(),
			},
		},
		"required":             []any{"revision", "items"},
		"additionalProperties": false,
	}
}

func todoUpdateOutputSchema() map[string]any {
	count := func() map[string]any { return map[string]any{"type": "integer", "minimum": 0} }
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id": map[string]any{"type": "string"},
			"revision":   count(), "total": count(), "pending": count(), "in_progress": count(), "completed": count(),
			"changes": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"added": count(), "removed": count(), "updated": count(), "started": count(), "completed": count(), "reopened": count(),
				},
				"required":             []any{"added", "removed", "updated", "started", "completed", "reopened"},
				"additionalProperties": false,
			},
		},
		"required":             []any{"revision", "total", "pending", "in_progress", "completed", "changes"},
		"additionalProperties": false,
	}
}
