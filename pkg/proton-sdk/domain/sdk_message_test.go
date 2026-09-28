package domain_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/usecase"
)

func TestCloneMessagesCopiesToolArguments(t *testing.T) {
	original := []domain.Message{{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}}}}
	clone := domain.CloneMessages(original)
	clone[0].ToolCalls[0].Arguments[0] = 'X'
	if string(original[0].ToolCalls[0].Arguments) != `{"path":"README.md"}` {
		t.Fatalf("original arguments changed: %q", original[0].ToolCalls[0].Arguments)
	}
}

func TestSDKMessageTextContent(t *testing.T) {
	message := domain.Message{Role: domain.RoleUser, Parts: []domain.ContentPart{{Type: domain.ContentPartText, Text: "one"}, {Type: domain.ContentPartImage, MIMEType: "image/png", Data: "data"}, {Type: domain.ContentPartText, Text: "two"}}}
	if got := message.TextContent(); got != "one\ntwo" {
		t.Fatalf("TextContent() = %q", got)
	}
}

func TestMessageIDsAreStableAndOpaque(t *testing.T) {
	first := domain.NewMessageID()
	second := domain.NewMessageID()
	if first == "" || second == "" || first == second {
		t.Fatalf("message ids = %q, %q", first, second)
	}
	if !domain.ValidMessageID(first) || !domain.ValidMessageID(second) {
		t.Fatalf("generated message ids are invalid: %q %q", first, second)
	}
}

func TestEnsureMessageIDsPreservesExistingIdentity(t *testing.T) {
	messages := []domain.Message{{ID: "msg_existing", Role: domain.RoleUser, Content: "one"}, {Role: domain.RoleAssistant, Content: "two"}}
	first := domain.EnsureMessageIDs(messages)
	second := domain.EnsureMessageIDs(first)
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
	if err := (domain.Message{ID: "bad id", Role: domain.RoleUser}).Validate(); !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatalf("invalid id error = %v", err)
	}
	if err := (domain.Message{Role: domain.RoleUser}).Validate(); err != nil {
		t.Fatalf("legacy message without id rejected: %v", err)
	}
}

func TestToolMessageRequiresToolCallID(t *testing.T) {
	err := (domain.Message{Role: domain.RoleTool, ToolName: "read", Content: "ok"}).Validate()
	if err == nil || !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatalf("error = %v, want invalid request", err)
	}
}

func TestAppendAssistantStepCopiesToolCalls(t *testing.T) {
	args := json.RawMessage(`{"path":"README.md"}`)
	base := []domain.Message{{Role: domain.RoleUser, Content: "inspect"}}
	got := usecase.AppendAssistantStep(base, domain.StepResult{Text: "checking", ToolCalls: []domain.ToolCall{{ID: "call-1", Name: "read", Arguments: args}}})
	if len(got) != 2 || got[1].Role != domain.RoleAssistant || got[1].Content != "checking" || len(got[1].ToolCalls) != 1 {
		t.Fatalf("unexpected history: %#v", got)
	}
	got[1].ToolCalls[0].Arguments[0] = 'X'
	if string(args) != `{"path":"README.md"}` {
		t.Fatalf("source arguments mutated: %q", args)
	}
}

func TestSDKAppendToolResults(t *testing.T) {
	got, err := usecase.AppendToolResults(nil, []domain.ToolResult{{ToolCallID: "call-1", ToolName: "read", Content: "failed", IsError: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Role != domain.RoleTool || got[0].ToolCallID != "call-1" || got[0].ToolName != "read" || got[0].Content != "failed" || !got[0].ToolResultIsError {
		t.Fatalf("unexpected history: %#v", got)
	}
}

func TestAppendToolResultsRejectsInvalidResult(t *testing.T) {
	if _, err := usecase.AppendToolResults(nil, []domain.ToolResult{{ToolName: "read"}}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRequestValidatesReasoningEffort(t *testing.T) {
	for _, effort := range []domain.ReasoningEffort{
		domain.ReasoningDefault, domain.ReasoningNone, domain.ReasoningMinimal, domain.ReasoningLow, domain.ReasoningMedium,
		domain.ReasoningHigh, domain.ReasoningXHigh, domain.ReasoningMax,
	} {
		req := domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}, Options: domain.ModelOptions{ReasoningEffort: effort}}
		if err := req.Validate(); err != nil {
			t.Fatalf("Validate(%q) error = %v", effort, err)
		}
	}
	req := domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}, Options: domain.ModelOptions{ReasoningEffort: "turbo"}}
	if err := req.Validate(); err == nil {
		t.Fatal("Validate(turbo) error = nil")
	}
}

func TestSDKParseReasoningEffort(t *testing.T) {
	for input, want := range map[string]domain.ReasoningEffort{
		"auto": domain.ReasoningDefault, "DEFAULT": domain.ReasoningDefault, "minimal": domain.ReasoningMinimal, "low": domain.ReasoningLow,
		"medium": domain.ReasoningMedium, "high": domain.ReasoningHigh, "xhigh": domain.ReasoningXHigh, "max": domain.ReasoningMax,
	} {
		got, err := domain.ParseReasoningEffort(input)
		if err != nil || got != want {
			t.Fatalf("ParseReasoningEffort(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	if _, err := domain.ParseReasoningEffort("turbo"); err == nil {
		t.Fatal("ParseReasoningEffort(turbo) error = nil")
	}
}
