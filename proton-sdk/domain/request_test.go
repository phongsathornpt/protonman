package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestRequestValidation(t *testing.T) {
	valid := domain.Request{
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}},
		Tools:    []domain.Tool{{Name: "read", Description: "reads file"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid request failed: %v", err)
	}

	negTokens := valid
	negTokens.Options.MaxOutputTokens = -1
	if err := negTokens.Validate(); err == nil {
		t.Fatal("negative max tokens should fail")
	}

	badReasoning := valid
	badReasoning.Options.ReasoningEffort = "super"
	if err := badReasoning.Validate(); err == nil {
		t.Fatal("invalid reasoning should fail")
	}

	noMsgs := valid
	noMsgs.Messages = nil
	if err := noMsgs.Validate(); err == nil {
		t.Fatal("empty messages should fail")
	}

	badToolChoice := valid
	badToolChoice.Options.ToolChoice = "none"
	if err := badToolChoice.Validate(); err == nil {
		t.Fatal("bad tool choice should fail")
	}

	reqToolNoTools := valid
	reqToolNoTools.Tools = nil
	reqToolNoTools.Options.ToolChoice = domain.ToolChoiceRequired
	if err := reqToolNoTools.Validate(); err == nil {
		t.Fatal("required tool choice with no tools should fail")
	}
}

func TestToolCallValidation(t *testing.T) {
	call := domain.ToolCall{ID: "c1", Name: "read", Arguments: json.RawMessage(`{"path":"test"}`)}
	if err := call.Validate(); err != nil {
		t.Fatalf("valid tool call failed: %v", err)
	}

	noID := call
	noID.ID = ""
	if err := noID.Validate(); err == nil {
		t.Fatal("no ID should fail")
	}

	noName := call
	noName.Name = ""
	if err := noName.Validate(); err == nil {
		t.Fatal("no Name should fail")
	}

	badJSON := call
	badJSON.Arguments = json.RawMessage(`{not_json`)
	if err := badJSON.Validate(); err == nil {
		t.Fatal("invalid JSON arguments should fail")
	}

	cloned := call.Clone()
	cloned.Arguments[0] = '['
	if string(call.Arguments) != `{"path":"test"}` {
		t.Fatal("original arguments mutated")
	}
}

func TestToolValidation(t *testing.T) {
	tool := domain.Tool{Name: "search", Description: "searches"}
	if err := tool.Validate(); err != nil {
		t.Fatalf("valid tool failed: %v", err)
	}

	noName := tool
	noName.Name = ""
	if err := noName.Validate(); err == nil {
		t.Fatal("no name should fail")
	}

	noDesc := tool
	noDesc.Description = ""
	if err := noDesc.Validate(); err == nil {
		t.Fatal("no desc should fail")
	}
}

func TestToolResultValidation(t *testing.T) {
	res := domain.ToolResult{ToolCallID: "c1", ToolName: "read", Content: "ok"}
	if err := res.Validate(); err != nil {
		t.Fatalf("valid tool result failed: %v", err)
	}

	noID := res
	noID.ToolCallID = ""
	if err := noID.Validate(); err == nil {
		t.Fatal("no ID should fail")
	}

	noName := res
	noName.ToolName = ""
	if err := noName.Validate(); err == nil {
		t.Fatal("no Name should fail")
	}
}
