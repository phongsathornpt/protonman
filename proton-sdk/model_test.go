package protonsdk

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRequestValidate(t *testing.T) {
	validTool := Tool{Name: "read", Description: "read a file"}
	tests := []struct {
		name    string
		request Request
		wantErr error
	}{
		{name: "valid", request: Request{Messages: []Message{{Role: RoleUser, Content: "inspect"}}, Tools: []Tool{validTool}}},
		{name: "missing messages", request: Request{Tools: []Tool{validTool}}, wantErr: ErrInvalidRequest},
		{name: "unknown role", request: Request{Messages: []Message{{Role: "provider"}}}, wantErr: ErrInvalidRequest},
		{name: "invalid tool", request: Request{Messages: []Message{{Role: RoleUser}}, Tools: []Tool{{Name: "read"}}}, wantErr: ErrInvalidRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if tt.wantErr == nil && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

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

func TestAgentStreamEventLifecycle(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantErr bool
	}{
		{name: "text start", event: Event{Kind: EventTextStart}},
		{name: "text delta", event: Event{Kind: EventTextDelta, Text: "hi"}},
		{name: "text end", event: Event{Kind: EventTextEnd}},
		{name: "tool start", event: Event{Kind: EventToolCallStart, ToolCallID: "call-1", ToolName: "read"}},
		{name: "tool delta", event: Event{Kind: EventToolCallDelta, ToolCallID: "call-1", ArgumentsDelta: `{"path"`}},
		{name: "tool end", event: Event{Kind: EventToolCallEnd, ToolCallID: "call-1"}},
		{name: "complete tool call", event: Event{Kind: EventToolCall, ToolCall: ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
		{name: "missing tool id", event: Event{Kind: EventToolCallDelta}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate() error = nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestAgentToolContracts(t *testing.T) {
	tool := Tool{
		Name:        "mcp_lookup",
		Description: "look up a runtime resource",
		InputSchema: map[string]any{"type": "object"},
		Dynamic:     true,
	}
	if err := tool.Validate(); err != nil {
		t.Fatalf("Tool.Validate() error = %v", err)
	}
	result := ToolResult{ToolCallID: "call-1", ToolName: tool.Name, Content: "ok"}
	if err := result.Validate(); err != nil {
		t.Fatalf("ToolResult.Validate() error = %v", err)
	}
	if err := (ToolResult{ToolName: tool.Name}).Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("missing call id error = %v", err)
	}
}

func TestUsageAndFinishEvents(t *testing.T) {
	if err := (Event{Kind: EventUsage, Usage: Usage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12}}).Validate(); err != nil {
		t.Fatalf("usage event error = %v", err)
	}
	if err := (Event{Kind: EventUsage, Usage: Usage{InputTokens: -1}}).Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("negative usage error = %v", err)
	}
	if err := (Event{Kind: EventFinish, FinishReason: FinishToolCalls}).Validate(); err != nil {
		t.Fatalf("finish event error = %v", err)
	}
	if err := (Event{Kind: EventFinish, FinishReason: "provider_magic"}).Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("unknown finish error = %v", err)
	}
}

func TestRawEventValidation(t *testing.T) {
	if err := (Event{Kind: EventRaw, RawData: []byte(`{"ok":true}`)}).Validate(); err != nil {
		t.Fatalf("raw event error = %v", err)
	}
	if err := (Event{Kind: EventRaw}).Validate(); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("empty raw event error = %v", err)
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
