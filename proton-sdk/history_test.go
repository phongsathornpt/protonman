package protonsdk

import (
	"encoding/json"
	"testing"
)

func TestAppendAssistantStepCopiesToolCalls(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md"}`)
	base := []Message{{Role: RoleUser, Content: "inspect"}}
	got := AppendAssistantStep(base, StepResult{Text: "checking", ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file", Arguments: args}}})
	if len(got) != 2 || got[1].Role != RoleAssistant || got[1].Content != "checking" || len(got[1].ToolCalls) != 1 {
		t.Fatalf("unexpected history: %#v", got)
	}
	got[1].ToolCalls[0].Arguments[0] = 'X'
	if string(args) != `{"path":"README.md"}` {
		t.Fatalf("source arguments mutated: %q", args)
	}
}

func TestAppendToolResults(t *testing.T) {
	got, err := AppendToolResults(nil, []ToolResult{{ToolCallID: "call-1", ToolName: "read_file", Content: "failed", IsError: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Role != RoleTool || got[0].ToolCallID != "call-1" || got[0].ToolName != "read_file" || got[0].Content != "failed" || !got[0].ToolResultIsError {
		t.Fatalf("unexpected history: %#v", got)
	}
}

func TestAppendToolResultsRejectsInvalidResult(t *testing.T) {
	if _, err := AppendToolResults(nil, []ToolResult{{ToolName: "read_file"}}); err == nil {
		t.Fatal("expected validation error")
	}
}
