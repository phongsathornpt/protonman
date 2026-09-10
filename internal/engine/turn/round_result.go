package turn

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func finalizeMaxToolCallResponse(
	ctx context.Context,
	sink Sink,
	round int,
	assistant model.Message,
) (model.Message, error) {
	slog.DebugContext(ctx, "turn ignored tool calls after max tool calls",
		"round", round,
	)
	return finalizeDisabledToolCallResponse(ctx, sink, round, assistant, MaxToolCallsFallback, "max-tool-calls")
}

func finalizeNoProgressToolCallResponse(
	ctx context.Context,
	sink Sink,
	round int,
	assistant model.Message,
) (model.Message, error) {
	slog.DebugContext(ctx, "turn ignored tool calls after no-progress detection",
		"round", round,
	)
	return finalizeDisabledToolCallResponse(ctx, sink, round, assistant, NoProgressFallback, "no-progress")
}

func finalizeDisabledToolCallResponse(
	ctx context.Context,
	sink Sink,
	round int,
	assistant model.Message,
	addition string,
	label string,
) (model.Message, error) {
	ignoredToolCalls := len(assistant.ToolCalls)
	assistant.ToolCalls = nil
	if strings.TrimSpace(assistant.Content) != "" {
		addition = "\n\n" + addition
	}
	assistant.Content += addition
	slog.DebugContext(ctx, "turn appended disabled-tool fallback",
		"round", round,
		"ignored_tool_calls", ignoredToolCalls,
		"fallback", label,
	)
	if err := emit(ctx, sink, Event{
		Kind:  EventTextDelta,
		Round: round,
		Text:  addition,
	}); err != nil {
		return model.Message{}, fmt.Errorf("emit %s fallback: %w", label, err)
	}
	return assistant, nil
}

func toolMessagesForExecutions(executions []executedCall) ([]model.Message, error) {
	messages := make([]model.Message, 0, len(executions))
	for _, execution := range executions {
		toolResult := execution.result
		content, err := json.Marshal(toolResult.ModelPayload())
		if err != nil {
			return nil, fmt.Errorf("encode tool result %q: %w", execution.call.Name, err)
		}
		modelToolName := execution.modelToolName
		if modelToolName == "" {
			modelToolName = execution.call.Name
		}
		messages = append(messages, model.Message{
			ID:                model.NewMessageID(),
			Role:              model.RoleTool,
			Content:           string(content),
			ToolCallID:        execution.call.ID,
			ToolName:          modelToolName,
			ToolResultIsError: execution.err != nil || toolResult.Denied || toolResult.Failure != nil,
		})
	}
	return messages, nil
}
