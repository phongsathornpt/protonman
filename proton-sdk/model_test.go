package protonsdk

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRequestValidate(t *testing.T) {
	validTool := Tool{Name: "read_file", Description: "read a file"}
	tests := []struct {
		name    string
		request Request
		wantErr error
	}{
		{name: "valid", request: Request{Messages: []Message{{Role: RoleUser, Content: "inspect"}}, Tools: []Tool{validTool}}},
		{name: "missing messages", request: Request{Tools: []Tool{validTool}}, wantErr: ErrInvalidRequest},
		{name: "unknown role", request: Request{Messages: []Message{{Role: "provider"}}}, wantErr: ErrInvalidRequest},
		{name: "invalid tool", request: Request{Messages: []Message{{Role: RoleUser}}, Tools: []Tool{{Name: "read_file"}}}, wantErr: ErrInvalidRequest},
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
	original := []Message{{Role: RoleAssistant, ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}}}}
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
