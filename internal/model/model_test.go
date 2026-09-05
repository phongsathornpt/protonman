package model

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/projectTHORN/proton/internal/tool"
)

func TestRequestValidate(t *testing.T) {
	validTool := tool.Definition{
		Name:                "read_file",
		Description:         "read a file",
		Kind:                tool.KindRead,
		PermissionDetailKey: "path",
	}
	tests := []struct {
		name    string
		request Request
		wantErr error
	}{
		{
			name: "valid",
			request: Request{
				Messages: []Message{{Role: RoleUser, Content: "inspect the project"}},
				Tools:    []tool.Definition{validTool},
			},
		},
		{
			name:    "missing messages",
			request: Request{Tools: []tool.Definition{validTool}},
			wantErr: ErrInvalidRequest,
		},
		{
			name:    "unknown role",
			request: Request{Messages: []Message{{Role: "provider"}}},
			wantErr: ErrInvalidRequest,
		},
		{
			name: "tool call on user message",
			request: Request{Messages: []Message{{
				Role: RoleUser,
				ToolCalls: []ToolCall{{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"README.md"}`),
				}},
			}}},
			wantErr: ErrInvalidRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.request.Validate()
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() error = %v, want errors.Is(..., %v)", err, test.wantErr)
			}
		})
	}
}

func TestCloneMessagesCopiesToolArguments(t *testing.T) {
	original := []Message{{
		Role: RoleAssistant,
		ToolCalls: []ToolCall{{
			ID:        "call-1",
			Name:      "read_file",
			Arguments: json.RawMessage(`{"path":"README.md"}`),
		}},
	}}
	clone := CloneMessages(original)
	clone[0].ToolCalls[0].Arguments[0] = 'X'
	if string(original[0].ToolCalls[0].Arguments) != `{"path":"README.md"}` {
		t.Fatalf("original tool arguments changed to %q", original[0].ToolCalls[0].Arguments)
	}
}

func TestEventValidate(t *testing.T) {
	tests := []struct {
		name    string
		event   Event
		wantErr error
	}{
		{name: "text", event: Event{Kind: EventTextDelta}},
		{name: "done", event: Event{Kind: EventDone}},
		{
			name: "tool call",
			event: Event{
				Kind: EventToolCall,
				ToolCall: ToolCall{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"README.md"}`),
				},
			},
		},
		{name: "unknown", event: Event{Kind: "unknown"}, wantErr: ErrInvalidEvent},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.event.Validate()
			if test.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() error = %v, want errors.Is(..., %v)", err, test.wantErr)
			}
		})
	}
}

func TestContentPartsAndTextContent(t *testing.T) {
	msg1 := Message{Role: RoleUser, Content: "direct content"}
	if msg1.TextContent() != "direct content" {
		t.Fatalf("TextContent() = %q, want %q", msg1.TextContent(), "direct content")
	}

	msg2 := Message{
		Role: RoleUser,
		Parts: []ContentPart{
			{Type: ContentPartText, Text: "part 1"},
			{Type: ContentPartImage, MIMEType: "image/png", Data: "iVBORw0KGgo="},
			{Type: ContentPartText, Text: "part 2"},
		},
	}
	if msg2.TextContent() != "part 1\npart 2" {
		t.Fatalf("TextContent() = %q, want %q", msg2.TextContent(), "part 1\npart 2")
	}

	cloned := CloneMessages([]Message{msg2})
	if len(cloned[0].Parts) != 3 {
		t.Fatalf("cloned Parts len = %d, want 3", len(cloned[0].Parts))
	}
	if cloned[0].Parts[1].Data != "iVBORw0KGgo=" {
		t.Fatalf("cloned Part[1].Data = %q, want %q", cloned[0].Parts[1].Data, "iVBORw0KGgo=")
	}
}

