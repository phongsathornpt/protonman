package protonsdk

// AppendAssistantStep appends the normalized assistant output of one model step
// to conversation history. Text and tool calls stay on the same logical
// assistant message; provider adapters may split them on the wire if required.
func AppendAssistantStep(messages []Message, result StepResult) []Message {
	next := CloneMessages(messages)
	if result.Text == "" && len(result.ToolCalls) == 0 {
		return next
	}
	assistant := Message{
		Role:      RoleAssistant,
		Content:   result.Text,
		ToolCalls: make([]ToolCall, 0, len(result.ToolCalls)),
	}
	for _, call := range result.ToolCalls {
		assistant.ToolCalls = append(assistant.ToolCalls, cloneToolCall(call))
	}
	return append(next, assistant)
}

// AppendToolResults appends tool execution outputs in model-history order.
func AppendToolResults(messages []Message, results []ToolResult) ([]Message, error) {
	next := CloneMessages(messages)
	for _, result := range results {
		if err := result.Validate(); err != nil {
			return nil, err
		}
		next = append(next, Message{
			Role:              RoleTool,
			Content:           result.Content,
			Parts:             append([]ContentPart(nil), result.Parts...),
			ToolCallID:        result.ToolCallID,
			ToolName:          result.ToolName,
			ToolResultIsError: result.IsError,
		})
	}
	return next, nil
}
