package session

import (
	"strings"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestFromModelMessagesCanonicalizesLegacyToolNames(t *testing.T) {
	messages := []sdk.Message{
		{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{{ID: "call-1", Name: "read"}}},
		{Role: sdk.RoleTool, ToolCallID: "call-1", ToolName: "read", Content: "legacy output"},
	}
	persisted := FromModelMessages(messages)
	if len(persisted) != 1 {
		t.Fatalf("persisted messages = %#v", persisted)
	}
	if strings.Contains(persisted[0].Content, "read") {
		t.Fatalf("legacy tool leaked into persisted history: %q", persisted[0].Content)
	}
	if !strings.Contains(persisted[0].Content, "Historical tool read result") {
		t.Fatalf("canonical tool missing from persisted history: %q", persisted[0].Content)
	}
}
