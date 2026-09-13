package protonsdk

// AppendAssistantResponse appends a normalized assistant response to conversation
// history. Text and tool calls stay on the same logical assistant message;
// provider adapters may split them on the wire if required.
func AppendAssistantResponse(messages []Message, response Response) []Message {
	next := CloneMessages(messages)
	if response.Text == "" && len(response.ToolCalls) == 0 {
		return next
	}
	assistant := Message{
		ID:        NewMessageID(),
		Role:      RoleAssistant,
		Content:   response.Text,
		ToolCalls: make([]ToolCall, 0, len(response.ToolCalls)),
	}
	for _, call := range response.ToolCalls {
		assistant.ToolCalls = append(assistant.ToolCalls, cloneToolCall(call))
	}
	return append(next, assistant)
}

// AppendAssistantStep is retained for source compatibility with earlier SDK releases.
// Deprecated: use AppendAssistantResponse.
func AppendAssistantStep(messages []Message, result StepResult) []Message {
	return AppendAssistantResponse(messages, result)
}

// AppendToolResults appends tool execution outputs in model-history order.
func AppendToolResults(messages []Message, results []ToolResult) ([]Message, error) {
	next := CloneMessages(messages)
	for _, result := range results {
		if err := result.Validate(); err != nil {
			return nil, err
		}
		next = append(next, Message{
			ID:                NewMessageID(),
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
