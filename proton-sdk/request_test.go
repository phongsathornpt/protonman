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

func TestRequestToolCallValidationUsesInvalidRequest(t *testing.T) {
	err := (Request{Messages: []Message{{Role: RoleAssistant, ToolCalls: []ToolCall{{Name: "read", Arguments: json.RawMessage(`{}`)}}}}}).Validate()
	if err == nil || !errors.Is(err, ErrInvalidRequest) || errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("error = %v, want only invalid request", err)
	}
}

func TestRequestRejectsNegativeMaxOutputTokens(t *testing.T) {
	req := Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}, Options: ModelOptions{MaxOutputTokens: -1}}
	if err := req.Validate(); err == nil {
		t.Fatal("expected validation error")
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

func TestToolCallClone(t *testing.T) {
	original := ToolCall{
		ID:        "call-1",
		Name:      "read",
		Arguments: json.RawMessage(`{"path":"main.go"}`),
	}
	cloned := original.Clone()
	if cloned.ID != original.ID || cloned.Name != original.Name {
		t.Fatalf("cloned tool call mismatch: %#v vs %#v", cloned, original)
	}
	cloned.Arguments[0] = '['
	if string(original.Arguments) != `{"path":"main.go"}` {
		t.Fatalf("original arguments mutated: %s", original.Arguments)
	}
}

func TestValidateToolInput(t *testing.T) {
	tool := Tool{Name: "read", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		// []string intentionally exercises schema normalization from ordinary Go values.
		"required":             []string{"path"},
		"additionalProperties": false,
	}}
	if err := ValidateToolInput(tool, json.RawMessage(`{"path":"README.md"}`)); err != nil {
		t.Fatalf("valid input error = %v", err)
	}
	for _, input := range []json.RawMessage{json.RawMessage(`{}`), json.RawMessage(`{"path":"README.md","extra":true}`)} {
		if err := ValidateToolInput(tool, input); !errors.Is(err, ErrInvalidToolInput) {
			t.Fatalf("invalid input %s error = %v", input, err)
		}
	}
}

func TestValidateToolOutput(t *testing.T) {
	tool := Tool{Name: "structured", OutputSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"count": map[string]any{"type": "number"}},
		// []string intentionally exercises schema normalization from ordinary Go values.
		"required": []string{"count"},
	}}
	if err := ValidateToolOutput(tool, json.RawMessage(`{"count":2}`)); err != nil {
		t.Fatalf("valid output error = %v", err)
	}
	if err := ValidateToolOutput(tool, json.RawMessage(`{"count":"two"}`)); !errors.Is(err, ErrInvalidToolOutput) {
		t.Fatalf("invalid output error = %v", err)
	}
	if err := ValidateToolOutput(tool, nil); !errors.Is(err, ErrInvalidToolOutput) {
		t.Fatalf("missing output error = %v", err)
	}
}

func TestToolSchemaValidationRejectsExternalRefs(t *testing.T) {
	inputTool := Tool{Name: "remote-input", InputSchema: map[string]any{"$ref": "https://example.com/schema.json"}}
	if err := ValidateToolInput(inputTool, json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidToolInput) {
		t.Fatalf("external input ref error = %v", err)
	}
	outputTool := Tool{Name: "remote-output", OutputSchema: map[string]any{"$ref": "https://example.com/schema.json"}}
	if err := ValidateToolOutput(outputTool, json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidToolOutput) {
		t.Fatalf("external output ref error = %v", err)
	}
}

func TestCompileToolSchemaRejectsMalformedSchema(t *testing.T) {
	tool := Tool{Name: "broken", InputSchema: map[string]any{"type": "object", "required": "path"}}
	if _, err := CompileToolInputValidator(tool); !errors.Is(err, ErrInvalidToolInput) {
		t.Fatalf("CompileToolInputValidator() error = %v, want ErrInvalidToolInput", err)
	}
}
