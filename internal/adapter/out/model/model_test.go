package model

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	port "github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
	usecase "github.com/phongsathornpt/protonman/pkg/proton-sdk/usecase"
)

func TestCloneMessagesCopiesToolArguments(t *testing.T) {
	original := []Message{{
		Role:      RoleAssistant,
		ToolCalls: []ToolCall{{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
	}}
	clone := CloneMessages(original)
	clone[0].ToolCalls[0].Arguments[0] = 'X'
	if string(original[0].ToolCalls[0].Arguments) != `{"path":"README.md"}` {
		t.Fatalf("original tool arguments changed to %q", original[0].ToolCalls[0].Arguments)
	}
}

func TestSnapshotMessagesIsolatesTopLevelSliceOnly(t *testing.T) {
	original := []Message{{
		Role:      RoleAssistant,
		ToolCalls: []ToolCall{{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
	}}
	snapshot := SnapshotMessages(original)
	snapshot[0].Role = RoleUser
	if original[0].Role != RoleAssistant {
		t.Fatalf("top-level message mutated through snapshot: %#v", original[0])
	}
	if &snapshot[0].ToolCalls[0] != &original[0].ToolCalls[0] {
		t.Fatal("snapshot unexpectedly deep-copied immutable tool calls")
	}
}

func TestContentPartsAndTextContent(t *testing.T) {
	msg1 := Message{Role: RoleUser, Content: "direct content"}
	if msg1.TextContent() != "direct content" {
		t.Fatalf("TextContent() = %q, want %q", msg1.TextContent(), "direct content")
	}
	msg2 := Message{Role: RoleUser, Parts: []ContentPart{
		{Type: ContentPartText, Text: "part 1"},
		{Type: ContentPartImage, MIMEType: "image/png", Data: "iVBORw0KGgo="},
		{Type: ContentPartText, Text: "part 2"},
	}}
	if msg2.TextContent() != "part 1\npart 2" {
		t.Fatalf("TextContent() = %q, want %q", msg2.TextContent(), "part 1\npart 2")
	}
	cloned := CloneMessages([]Message{msg2})
	if len(cloned[0].Parts) != 3 || cloned[0].Parts[1].Data != "iVBORw0KGgo=" {
		t.Fatalf("cloned parts = %#v", cloned[0].Parts)
	}
}

type metadataTestModel struct{}

func (*metadataTestModel) Provider() string { return "test" }
func (*metadataTestModel) ModelID() string  { return "model" }
func (*metadataTestModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true}
}
func (*metadataTestModel) Metadata() domain.ModelMetadata {
	return domain.ModelMetadata{TokenLimits: domain.TokenLimits{
		ContextWindow: 1000, MaxInputTokens: 800, MaxOutputTokens: 200,
	}}
}
func (*metadataTestModel) Stream(context.Context, domain.Request) (port.Stream, error) {
	return nil, nil
}

func TestModelDecoratorsPreserveCanonicalMetadata(t *testing.T) {
	base := port.LanguageModel(&metadataTestModel{})
	wrapped := withSessionID(base, "session")
	wrapped = withContextWindow(wrapped, 1200)

	metadata := usecase.ModelMetadataOf(wrapped)
	if metadata.TokenLimits.ContextWindow != 1200 {
		t.Fatalf("context window = %d, want 1200", metadata.TokenLimits.ContextWindow)
	}
	if metadata.TokenLimits.MaxInputTokens != 800 || metadata.TokenLimits.MaxOutputTokens != 200 {
		t.Fatalf("token limits changed through decorators: %#v", metadata.TokenLimits)
	}
}

func BenchmarkCloneMessagesLargeToolHistory(b *testing.B) {
	messages := make([]Message, 0, 128)
	arguments := json.RawMessage(`{"payload":"` + strings.Repeat("x", 64*1024) + `"}`)
	for i := 0; i < 128; i++ {
		messages = append(messages, Message{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call", Name: "read", Arguments: arguments}}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = CloneMessages(messages)
	}
}

func BenchmarkSnapshotMessagesLargeToolHistory(b *testing.B) {
	messages := make([]Message, 0, 128)
	arguments := json.RawMessage(`{"payload":"` + strings.Repeat("x", 64*1024) + `"}`)
	for i := 0; i < 128; i++ {
		messages = append(messages, Message{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call", Name: "read", Arguments: arguments}}})
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = SnapshotMessages(messages)
	}
}
