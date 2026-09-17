package domain_test

import (
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func TestEffectiveRequestIDUsesExplicitMetadata(t *testing.T) {
	req := domain.Request{
		Messages: []domain.Message{{ID: "msg-user", Role: domain.RoleUser, Content: "hi"}},
		Metadata: domain.RequestMetadata{RequestID: " req-explicit "},
	}
	if got := req.EffectiveRequestID(); got != "req-explicit" {
		t.Fatalf("EffectiveRequestID() = %q, want %q", got, "req-explicit")
	}
}

func TestEffectiveRequestIDUsesLatestUserMessage(t *testing.T) {
	req := domain.Request{Messages: []domain.Message{
		{ID: "msg-user-1", Role: domain.RoleUser, Content: "first"},
		{ID: "msg-assistant", Role: domain.RoleAssistant, Content: "thinking"},
		{ID: "msg-user-2", Role: domain.RoleUser, Content: "second"},
		{ID: "msg-tool", Role: domain.RoleTool, Content: "result", ToolCallID: "call-1", ToolName: "read"},
	}}
	if got := req.EffectiveRequestID(); got != "msg-user-2" {
		t.Fatalf("EffectiveRequestID() = %q, want %q", got, "msg-user-2")
	}
}
