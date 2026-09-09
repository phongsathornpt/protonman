package conversation

import (
	"strings"
	"testing"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestRetainDropsOldestMessagesWithinCountLimit(t *testing.T) {
	messages := []sdk.Message{
		{Role: sdk.RoleUser, Content: "one"},
		{Role: sdk.RoleAssistant, Content: "two"},
		{Role: sdk.RoleUser, Content: "three"},
	}
	got := Retain(messages, RetentionPolicy{MaxMessages: 2})
	if len(got) != 2 || got[0].Content != "two" || got[1].Content != "three" {
		t.Fatalf("retained messages = %#v", got)
	}
}

func TestRetainKeepsToolProtocolGroupIntact(t *testing.T) {
	messages := []sdk.Message{
		{Role: sdk.RoleUser, Content: "old"},
		{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{{ID: "c1", Name: "read", Arguments: []byte(`{"path":"a"}`)}}},
		{Role: sdk.RoleTool, ToolCallID: "c1", ToolName: "read", Content: "result"},
		{Role: sdk.RoleUser, Content: "latest"},
	}
	got := Retain(messages, RetentionPolicy{MaxMessages: 3})
	if len(got) != 3 {
		t.Fatalf("retained len=%d, want 3: %#v", len(got), got)
	}
	if got[0].Role != sdk.RoleAssistant || got[1].Role != sdk.RoleTool || got[2].Content != "latest" {
		t.Fatalf("tool group was split: %#v", got)
	}
}

func TestRetainPreservesLeadingSystemMessages(t *testing.T) {
	messages := []sdk.Message{
		{Role: sdk.RoleSystem, Content: "custom instructions"},
		{Role: sdk.RoleUser, Content: "old"},
		{Role: sdk.RoleAssistant, Content: "middle"},
		{Role: sdk.RoleUser, Content: "latest"},
	}
	got := Retain(messages, RetentionPolicy{MaxMessages: 2})
	if len(got) != 2 || got[0].Role != sdk.RoleSystem || got[1].Content != "latest" {
		t.Fatalf("protected system history = %#v", got)
	}
}

func TestRetainEnforcesByteBudgetByDroppingWholeSpans(t *testing.T) {
	messages := []sdk.Message{
		{Role: sdk.RoleUser, Content: strings.Repeat("x", 1024)},
		{Role: sdk.RoleAssistant, Content: "keep"},
	}
	got := Retain(messages, RetentionPolicy{MaxBytes: 512})
	if len(got) != 1 || got[0].Content != "keep" {
		t.Fatalf("byte-bounded history = %#v", got)
	}
	if EstimatedBytes(got) > 512 {
		t.Fatalf("estimated bytes=%d, want <=512", EstimatedBytes(got))
	}
}

func TestRetainReturnsFreshTopLevelSlice(t *testing.T) {
	messages := []sdk.Message{{Role: sdk.RoleUser, Content: "one"}, {Role: sdk.RoleAssistant, Content: "two"}}
	got := Retain(messages, RetentionPolicy{})
	got[0].Content = "changed"
	if messages[0].Content != "one" {
		t.Fatalf("Retain reused top-level backing slice: %#v", messages)
	}
}
func TestRetainCompactsStaleToolGroups(t *testing.T) {
	messages := []sdk.Message{
		{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{{ID: "old-call", Name: "read", Arguments: []byte(`{"path":"large"}`)}}},
		{Role: sdk.RoleTool, ToolCallID: "old-call", ToolName: "read", Content: `{"call_id":"old-call","tool_name":"read","output":"` + strings.Repeat("x", 512) + `"}`},
		{Role: sdk.RoleUser, Content: "recent question"},
		{Role: sdk.RoleAssistant, Content: "recent answer"},
	}
	got := Retain(messages, RetentionPolicy{MaxMessages: 100, MaxBytes: 1 << 20, RecentMessages: 2, MaxHistoricalToolResultBytes: 128})
	if len(got) != 3 {
		t.Fatalf("compacted len=%d, want 3: %#v", len(got), got)
	}
	if got[0].Role != sdk.RoleAssistant || len(got[0].ToolCalls) != 0 || !strings.Contains(got[0].Content, "Historical tool read result") {
		t.Fatalf("stale tool group was not compacted: %#v", got[0])
	}
	if !strings.Contains(got[0].Content, "historical tool output truncated") || len(got[0].Content) > 128 {
		t.Fatalf("stale tool output was not bounded: len=%d content=%q", len(got[0].Content), got[0].Content)
	}
}

func TestRetainDoesNotSplitRecentToolGroupAtWindowBoundary(t *testing.T) {
	messages := []sdk.Message{
		{Role: sdk.RoleUser, Content: "old"},
		{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{{ID: "recent-call", Name: "grep", Arguments: []byte(`{"pattern":"x"}`)}}},
		{Role: sdk.RoleTool, ToolCallID: "recent-call", ToolName: "grep", Content: "recent result"},
		{Role: sdk.RoleUser, Content: "latest"},
	}
	got := Retain(messages, RetentionPolicy{MaxMessages: 100, MaxBytes: 1 << 20, RecentMessages: 2, MaxHistoricalToolResultBytes: 64})
	if len(got) != 4 {
		t.Fatalf("recent boundary changed message count=%d: %#v", len(got), got)
	}
	if got[1].Role != sdk.RoleAssistant || len(got[1].ToolCalls) != 1 || got[2].Role != sdk.RoleTool {
		t.Fatalf("recent tool protocol group was compacted or split: %#v", got)
	}
}
