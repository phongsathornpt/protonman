package questiontool

func inputSchema() map[string]any {
	return map[string]any{
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
		},
		"required":             []any{"question"},
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
			"selected_options": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Array of selected option strings if options were chosen.",
			},
		},
		"required":             []any{"status", "answer"},
		"additionalProperties": false,
	}
}
