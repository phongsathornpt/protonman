package usecase

import (
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

// AppendAssistantResponse appends a normalized assistant response to conversation
// history. Text and tool calls stay on the same logical assistant message;
// provider adapters may split them on the wire if required.
func AppendAssistantResponse(messages []domain.Message, response domain.Response) []domain.Message {
	next := domain.CloneMessages(messages)
	if response.Text == "" && len(response.ToolCalls) == 0 {
		return next
	}
	assistant := domain.Message{
		ID:               domain.NewMessageID(),
		Role:             domain.RoleAssistant,
		Content:          response.Text,
		ReasoningContent: response.ReasoningContent,
		ToolCalls:        make([]domain.ToolCall, 0, len(response.ToolCalls)),
	}
	for _, call := range response.ToolCalls {
		assistant.ToolCalls = append(assistant.ToolCalls, call.Clone())
	}
	return append(next, assistant)
}

// AppendAssistantStep is retained for source compatibility with earlier SDK releases.
// Deprecated: use AppendAssistantResponse.
func AppendAssistantStep(messages []domain.Message, result domain.StepResult) []domain.Message {
	return AppendAssistantResponse(messages, result)
}

// AppendToolResults appends tool execution outputs in model-history order.
func AppendToolResults(messages []domain.Message, results []domain.ToolResult) ([]domain.Message, error) {
	next := domain.CloneMessages(messages)
	for _, result := range results {
		if err := result.Validate(); err != nil {
			return nil, err
		}
		next = append(next, domain.Message{
			ID:                domain.NewMessageID(),
			Role:              domain.RoleTool,
			Content:           result.Content,
			Parts:             append([]domain.ContentPart(nil), result.Parts...),
			ToolCallID:        result.ToolCallID,
			ToolName:          result.ToolName,
			ToolResultIsError: result.IsError,
		})
	}
	return next, nil
}
