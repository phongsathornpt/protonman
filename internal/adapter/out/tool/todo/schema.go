package todotool

// patchOperationSchema builds the closed object schema for one patch operation.
// Field/kind compatibility (for example that remove accepts only id) is enforced
// by the domain patch engine, which reports the precise defect; the schema only
// pins the envelope and the value types.
func patchOperationSchema() map[string]any {
	properties := map[string]any{
		"op": map[string]any{
			"type":        "string",
			"enum":        []any{"add", "set_status", "set_text", "remove"},
			"description": "Patch operation kind.",
		},
		"id":     map[string]any{"type": "string", "description": "Stable task id from the current snapshot, or a new stable id when op=add."},
		"text":   map[string]any{"type": "string", "description": "Task text; required for op=add, used by op=set_text, rejected by op=remove."},
		"status": map[string]any{"type": "string", "enum": []any{"pending", "in_progress", "completed"}, "description": "Task status; required for op=add and op=set_status, rejected by op=remove."},
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             []any{"op", "id"},
		"additionalProperties": false,
	}
}

// todoUpdateInputSchema declares the patch contract for the update action. A
// strict oneOf cannot express "get takes nothing, update takes both fields", so
// the conditional requirement is expressed with if/then, which keeps validation
// errors attributable to a single JSON pointer.
func todoUpdateInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expectedRevision": map[string]any{"type": "integer", "minimum": 0, "description": "Revision from the latest todo action=get snapshot. Send the JSON number returned by get; stale revisions are rejected."},
			"operations": map[string]any{
				"type": "array", "minItems": 1, "maxItems": 256,
				"description": "JSON array of patch-operation objects, never a JSON-encoded string. Unmentioned tasks are preserved.",
				"items":       patchOperationSchema(),
			},
		},
		"required":             []any{"expectedRevision", "operations"},
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
			"sessionId":  map[string]any{"type": "string"},
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
			"sessionId":  map[string]any{"type": "string"},
			"session_id": map[string]any{"type": "string"},
			"revision":   count(), "total": count(), "pending": count(), "inProgress": count(), "in_progress": count(), "completed": count(),
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
