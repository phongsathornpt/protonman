package usecase_test

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

func TestAppendAssistantResponse(t *testing.T) {
	initial := []domain.Message{{Role: domain.RoleUser, Content: "hi"}}
	resp := domain.Response{
		Text:             "hello",
		ReasoningContent: "inspect first",
		ToolCalls: []domain.ToolCall{
			{ID: "c1", Name: "read", Arguments: json.RawMessage(`{"path":"a"}`)},
		},
	}
	history := usecase.AppendAssistantResponse(initial, resp)
	if len(history) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(history))
	}
	if history[1].Role != domain.RoleAssistant || history[1].Content != "hello" {
		t.Fatalf("unexpected assistant message: %+v", history[1])
	}
	if history[1].ReasoningContent != "inspect first" {
		t.Fatalf("reasoning content was not preserved: %+v", history[1])
	}
	if len(history[1].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(history[1].ToolCalls))
	}

	// Empty response appends nothing
	emptyHistory := usecase.AppendAssistantResponse(initial, domain.Response{})
	if len(emptyHistory) != 1 {
		t.Fatalf("empty response should not append message")
	}

	// AppendAssistantStep
	stepHistory := usecase.AppendAssistantStep(initial, resp)
	if len(stepHistory) != 2 {
		t.Fatalf("expected 2 messages from AppendAssistantStep")
	}
}

func TestAppendToolResults(t *testing.T) {
	initial := []domain.Message{{Role: domain.RoleUser, Content: "hi"}}
	results := []domain.ToolResult{
		{ToolCallID: "c1", ToolName: "read", Content: "file contents", IsError: false},
	}
	history, err := usecase.AppendToolResults(initial, results)
	if err != nil {
		t.Fatalf("AppendToolResults failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(history))
	}
	if history[1].Role != domain.RoleTool || history[1].Content != "file contents" || history[1].ToolCallID != "c1" {
		t.Fatalf("unexpected tool message: %+v", history[1])
	}

	invalidResults := []domain.ToolResult{
		{ToolCallID: "", ToolName: "read"},
	}
	if _, err := usecase.AppendToolResults(initial, invalidResults); err == nil {
		t.Fatal("expected error for invalid tool result")
	}
}
