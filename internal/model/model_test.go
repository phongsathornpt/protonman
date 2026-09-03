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
