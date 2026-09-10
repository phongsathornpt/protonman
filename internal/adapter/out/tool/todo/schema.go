package todotool

func todoUpdateInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expected_revision": map[string]any{"type": "integer", "minimum": 0, "description": "Revision from the latest todo action=get snapshot; stale revisions are rejected."},
			"operations": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 256,
				"description": "Ordered patch operations. Unmentioned tasks are preserved. Operation-specific fields are validated by the todo runtime.",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"op":     map[string]any{"type": "string", "enum": []any{"add", "set_status", "set_text", "remove"}},
						"id":     map[string]any{"type": "string"},
						"text":   map[string]any{"type": "string"},
						"status": map[string]any{"type": "string", "enum": []any{"pending", "in_progress", "completed"}},
					},
					"required":             []any{"op", "id"},
					"additionalProperties": false,
				},
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
