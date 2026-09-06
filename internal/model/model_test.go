package model

import (
	"encoding/json"
	"testing"
)

func TestCloneMessagesCopiesToolArguments(t *testing.T) {
	original := []Message{{
		Role:      RoleAssistant,
		ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
	}}
	clone := CloneMessages(original)
	clone[0].ToolCalls[0].Arguments[0] = 'X'
	if string(original[0].ToolCalls[0].Arguments) != `{"path":"README.md"}` {
		t.Fatalf("original tool arguments changed to %q", original[0].ToolCalls[0].Arguments)
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
