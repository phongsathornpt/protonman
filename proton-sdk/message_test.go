package protonsdk

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestCloneMessagesCopiesToolArguments(t *testing.T) {
	original := []Message{{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}}}}
	clone := CloneMessages(original)
	clone[0].ToolCalls[0].Arguments[0] = 'X'
	if string(original[0].ToolCalls[0].Arguments) != `{"path":"README.md"}` {
		t.Fatalf("original arguments changed: %q", original[0].ToolCalls[0].Arguments)
	}
}

func TestMessageTextContent(t *testing.T) {
	message := Message{Role: RoleUser, Parts: []ContentPart{{Type: ContentPartText, Text: "one"}, {Type: ContentPartImage, MIMEType: "image/png", Data: "data"}, {Type: ContentPartText, Text: "two"}}}
	if got := message.TextContent(); got != "one\ntwo" {
		t.Fatalf("TextContent() = %q", got)
	}
}

func TestMessageIDsAreStableAndOpaque(t *testing.T) {
	first := NewMessageID()
	second := NewMessageID()
	if first == "" || second == "" || first == second {
		t.Fatalf("message ids = %q, %q", first, second)
	}
	if !ValidMessageID(first) || !ValidMessageID(second) {
		t.Fatalf("generated message ids are invalid: %q %q", first, second)
	}
}

func TestEnsureMessageIDsPreservesExistingIdentity(t *testing.T) {
	messages := []Message{{ID: "msg_existing", Role: RoleUser, Content: "one"}, {Role: RoleAssistant, Content: "two"}}
	first := EnsureMessageIDs(messages)
	second := EnsureMessageIDs(first)
	if first[0].ID != "msg_existing" || second[0].ID != "msg_existing" {
		t.Fatalf("existing id changed: first=%q second=%q", first[0].ID, second[0].ID)
	}
	if first[1].ID == "" || second[1].ID != first[1].ID {
		t.Fatalf("assigned id was not stable: first=%q second=%q", first[1].ID, second[1].ID)
	}
	if messages[1].ID != "" {
		t.Fatalf("EnsureMessageIDs mutated input: %q", messages[1].ID)
	}
}

func TestMessageValidateRejectsUnsafeIdentity(t *testing.T) {
	if err := (Message{ID: "bad id", Role: RoleUser}).Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid id error = %v", err)
	}
	if err := (Message{Role: RoleUser}).Validate(); err != nil {
		t.Fatalf("legacy message without id rejected: %v", err)
	}
}

func TestToolMessageRequiresToolCallID(t *testing.T) {
	err := (Message{Role: RoleTool, ToolName: "read", Content: "ok"}).Validate()
	if err == nil || !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("error = %v, want invalid request", err)
	}
}

func TestAppendAssistantStepCopiesToolCalls(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md"}`)
	base := []Message{{Role: RoleUser, Content: "inspect"}}
	got := AppendAssistantStep(base, StepResult{Text: "checking", ToolCalls: []ToolCall{{ID: "call-1", Name: "read", Arguments: args}}})
	if len(got) != 2 || got[1].Role != RoleAssistant || got[1].Content != "checking" || len(got[1].ToolCalls) != 1 {
		t.Fatalf("unexpected history: %#v", got)
	}
	got[1].ToolCalls[0].Arguments[0] = 'X'
	if string(args) != `{"path":"README.md"}` {
		t.Fatalf("source arguments mutated: %q", args)
	}
}

func TestAppendToolResults(t *testing.T) {
	got, err := AppendToolResults(nil, []ToolResult{{ToolCallID: "call-1", ToolName: "read", Content: "failed", IsError: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Role != RoleTool || got[0].ToolCallID != "call-1" || got[0].ToolName != "read" || got[0].Content != "failed" || !got[0].ToolResultIsError {
		t.Fatalf("unexpected history: %#v", got)
	}
}

func TestAppendToolResultsRejectsInvalidResult(t *testing.T) {
	if _, err := AppendToolResults(nil, []ToolResult{{ToolName: "read"}}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRequestValidatesReasoningEffort(t *testing.T) {
	for _, effort := range []ReasoningEffort{
		ReasoningDefault, ReasoningNone, ReasoningMinimal, ReasoningLow, ReasoningMedium,
		ReasoningHigh, ReasoningXHigh, ReasoningMax,
	} {
		req := Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}, Options: ModelOptions{ReasoningEffort: effort}}
		if err := req.Validate(); err != nil {
			t.Fatalf("Validate(%q) error = %v", effort, err)
		}
	}
	req := Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}, Options: ModelOptions{ReasoningEffort: "turbo"}}
	if err := req.Validate(); err == nil {
		t.Fatal("Validate(turbo) error = nil")
	}
}

func TestParseReasoningEffort(t *testing.T) {
	for input, want := range map[string]ReasoningEffort{
		"auto": ReasoningDefault, "DEFAULT": ReasoningDefault, "minimal": ReasoningMinimal, "low": ReasoningLow,
		"medium": ReasoningMedium, "high": ReasoningHigh, "xhigh": ReasoningXHigh, "max": ReasoningMax,
	} {
		got, err := ParseReasoningEffort(input)
		if err != nil || got != want {
			t.Fatalf("ParseReasoningEffort(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := ParseReasoningEffort("turbo"); err == nil {
		t.Fatal("ParseReasoningEffort(turbo) error = nil")
	}
}
