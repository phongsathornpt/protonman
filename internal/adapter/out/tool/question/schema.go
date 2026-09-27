package questiontool

func inputSchema() map[string]any {
	questionItemSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "The question to ask the user.",
			},
			"options": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Optional list of predefined choices for the user to select from.",
			},
			"multiple": map[string]any{
				"type":        "boolean",
				"description": "Whether the user can select multiple options (default: false).",
			},
			"recommended": map[string]any{
				"type":        "string",
				"description": "Optional recommended option for this question.",
			},
		},
		"required":             []any{"question"},
		"additionalProperties": false,
	}

	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "The question to ask the user (for single-question calls).",
			},
			"options": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Optional list of predefined choices for the user to select from.",
			},
			"multiple": map[string]any{
				"type":        "boolean",
				"description": "Whether the user can select multiple options (default: false).",
			},
			"recommended": map[string]any{
				"type":        "string",
				"description": "Optional recommended option.",
			},
			"questions": map[string]any{
				"type":        "array",
				"items":       questionItemSchema,
				"description": "Optional list of questions for multi-question interview rounds.",
			},
		},
		"additionalProperties": false,
	}
}

func outputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status": map[string]any{
				"type":        "string",
				"enum":        []any{string(StatusAnswered), string(StatusDeclined)},
				"description": "Whether the user provided an answer or declined/dismissed the prompt.",
			},
			"answer": map[string]any{
				"type":        "string",
				"description": "The user's response text or selected choices.",
			},
			"selectedOptions": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Array of selected option strings if options were chosen.",
			},
			"answers": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question": map[string]any{"type": "string"},
						"answer":   map[string]any{"type": "string"},
						"selectedOptions": map[string]any{
							"type":  "array",
							"items": map[string]any{"type": "string"},
						},
					},
					"required":             []any{"question", "answer"},
					"additionalProperties": false,
				},
				"description": "List of answers for each question in a multi-question round.",
			},
		},
		"required":             []any{"status", "answer"},
		"additionalProperties": false,
	}
}
