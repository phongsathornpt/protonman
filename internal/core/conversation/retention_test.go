package conversation

import (
	"fmt"
	"runtime"
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

func TestHistoricalToolResultFastPathDecodesEscapedOutput(t *testing.T) {
	content := `{"call_id":"c1","tool_name":"read","output":"line 1\nquoted: \"hello\"\tไทย"}`
	got := historicalToolResultText("read", content, 64*1024)
	want := "Historical tool read result:\nline 1\nquoted: \"hello\"\tไทย"
	if got != want {
		t.Fatalf("historical output = %q, want %q", got, want)
	}
}

func TestHistoricalToolResultFastPathHonorsByteLimit(t *testing.T) {
	const limit = 96
	content := `{"call_id":"c1","tool_name":"read","output":"` + strings.Repeat("x", 512) + `"}`
	got := historicalToolResultText("read", content, limit)
	if len(got) > limit {
		t.Fatalf("historical output bytes=%d, want <=%d", len(got), limit)
	}
	if !strings.Contains(got, "[historical tool output truncated]") {
		t.Fatalf("historical output missing truncation marker: %q", got)
	}
}

func TestHistoricalToolResultFallsBackForFailurePayload(t *testing.T) {
	content := `{"call_id":"c1","tool_name":"read","error":{"code":"execution_error","message":"boom"}}`
	got := historicalToolResultText("read", content, 64*1024)
	if got != "Historical tool read failed [execution_error]: boom" {
		t.Fatalf("historical failure = %q", got)
	}
}

func BenchmarkRetainLongToolHeavyConversation(b *testing.B) {
	messages := make([]sdk.Message, 0, 800)
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("call-%d", i)
		messages = append(messages,
			sdk.Message{Role: sdk.RoleUser, Content: fmt.Sprintf("question-%d", i)},
			sdk.Message{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{{ID: id, Name: "read", Arguments: []byte(`{"path":"large.log"}`)}}},
			sdk.Message{Role: sdk.RoleTool, ToolCallID: id, ToolName: "read", Content: `{"call_id":"` + id + `","tool_name":"read","output":"` + strings.Repeat("x", 8*1024) + `"}`},
			sdk.Message{Role: sdk.RoleAssistant, Content: "done"},
		)
	}
	policy := DefaultRetentionPolicy()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		retained := Retain(messages, policy)
		if len(retained) > policy.MaxMessages {
			b.Fatalf("retained %d messages > %d", len(retained), policy.MaxMessages)
		}
	}
}

func BenchmarkRetainRecentConversation(b *testing.B) {
	messages := make([]sdk.Message, 0, 64)
	for i := 0; i < 64; i++ {
		messages = append(messages, sdk.Message{Role: sdk.RoleUser, Content: fmt.Sprintf("message-%d", i)})
	}
	policy := DefaultRetentionPolicy()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Retain(messages, policy)
	}
}

func TestLongSessionRetentionBoundsRetainedPayload(t *testing.T) {
	policy := RetentionPolicy{
		MaxMessages:                  128,
		MaxBytes:                     2 * 1024 * 1024,
		RecentMessages:               16,
		MaxHistoricalToolResultBytes: 4 * 1024,
	}
	var messages []sdk.Message
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("call-%d", i)
		messages = append(messages,
			sdk.Message{Role: sdk.RoleUser, Content: fmt.Sprintf("question-%d", i)},
			sdk.Message{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{{ID: id, Name: "read", Arguments: []byte(`{"path":"large.log"}`)}}},
			sdk.Message{Role: sdk.RoleTool, ToolCallID: id, ToolName: "read", Content: `{"call_id":"` + id + `","tool_name":"read","output":"` + strings.Repeat("x", 128*1024) + `"}`},
			sdk.Message{Role: sdk.RoleAssistant, Content: "done"},
		)
		messages = Retain(messages, policy)
	}
	if len(messages) > policy.MaxMessages {
		t.Fatalf("retained messages=%d, want <=%d", len(messages), policy.MaxMessages)
	}
	if got := EstimatedBytes(messages); got > policy.MaxBytes {
		t.Fatalf("retained payload=%d bytes, want <=%d", got, policy.MaxBytes)
	}
}

func TestLongSessionRetentionBoundsHeapGrowth(t *testing.T) {
	policy := RetentionPolicy{
		MaxMessages:                  96,
		MaxBytes:                     4 * 1024 * 1024,
		RecentMessages:               8,
		MaxHistoricalToolResultBytes: 8 * 1024,
	}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	var messages []sdk.Message
	for i := 0; i < 96; i++ {
		id := fmt.Sprintf("heap-call-%d", i)
		messages = append(messages,
			sdk.Message{Role: sdk.RoleUser, Content: "inspect"},
			sdk.Message{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{{ID: id, Name: "read", Arguments: []byte(`{"path":"heap.log"}`)}}},
			sdk.Message{Role: sdk.RoleTool, ToolCallID: id, ToolName: "read", Content: `{"call_id":"` + id + `","tool_name":"read","output":"` + strings.Repeat("y", 256*1024) + `"}`},
			sdk.Message{Role: sdk.RoleAssistant, Content: "done"},
		)
		messages = Retain(messages, policy)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(messages)

	const maxGrowth = 10 * 1024 * 1024
	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if growth > maxGrowth {
		t.Fatalf("retained heap grew by %d bytes after bounded session; want <=%d", growth, maxGrowth)
	}
}
