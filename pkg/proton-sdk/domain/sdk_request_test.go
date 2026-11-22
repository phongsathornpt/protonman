package domain_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/usecase"
)

func TestRequestValidate(t *testing.T) {
	validTool := domain.Tool{Name: "read", Description: "read a file"}
	tests := []struct {
		name    string
		request domain.Request
		wantErr error
	}{
		{name: "valid", request: domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "inspect"}}, Tools: []domain.Tool{validTool}}},
		{name: "missing messages", request: domain.Request{Tools: []domain.Tool{validTool}}, wantErr: domain.ErrInvalidRequest},
		{name: "unknown role", request: domain.Request{Messages: []domain.Message{{Role: "provider"}}}, wantErr: domain.ErrInvalidRequest},
		{name: "invalid tool", request: domain.Request{Messages: []domain.Message{{Role: domain.RoleUser}}, Tools: []domain.Tool{{Name: "read"}}}, wantErr: domain.ErrInvalidRequest},
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
	err := (domain.Request{Messages: []domain.Message{{Role: domain.RoleAssistant, ToolCalls: []domain.ToolCall{{Name: "read", Arguments: json.RawMessage(`{}`)}}}}}).Validate()
	if err == nil || !errors.Is(err, domain.ErrInvalidRequest) || errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("error = %v, want only invalid request", err)
	}
}

func TestRequestRejectsNegativeMaxOutputTokens(t *testing.T) {
	req := domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}, Options: domain.ModelOptions{MaxOutputTokens: -1}}
	if err := req.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestAgentToolContracts(t *testing.T) {
	tool := domain.Tool{
		Name:        "mcp_lookup",
		Description: "look up a runtime resource",
		InputSchema: map[string]any{"type": "object"},
		Dynamic:     true,
	}
	if err := tool.Validate(); err != nil {
		t.Fatalf("Tool.Validate() error = %v", err)
	}
	result := domain.ToolResult{ToolCallID: "call-1", ToolName: tool.Name, Content: "ok"}
	if err := result.Validate(); err != nil {
		t.Fatalf("ToolResult.Validate() error = %v", err)
	}
	if err := (domain.ToolResult{ToolName: tool.Name}).Validate(); !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatalf("missing call id error = %v", err)
	}
}

func TestToolCallClone(t *testing.T) {
	original := domain.ToolCall{
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
	tool := domain.Tool{Name: "read", InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		// []string intentionally exercises schema normalization from ordinary Go values.
		"required":             []string{"path"},
		"additionalProperties": false,
	}}
	if err := usecase.ValidateToolInput(tool, json.RawMessage(`{"path":"README.md"}`)); err != nil {
		t.Fatalf("valid input error = %v", err)
	}
	for _, input := range []json.RawMessage{json.RawMessage(`{}`), json.RawMessage(`{"path":"README.md","extra":true}`)} {
		if err := usecase.ValidateToolInput(tool, input); !errors.Is(err, domain.ErrInvalidToolInput) {
			t.Fatalf("invalid input %s error = %v", input, err)
		}
	}
}

func TestValidateToolOutput(t *testing.T) {
	tool := domain.Tool{Name: "structured", OutputSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"count": map[string]any{"type": "number"}},
		// []string intentionally exercises schema normalization from ordinary Go values.
		"required": []string{"count"},
	}}
	if err := usecase.ValidateToolOutput(tool, json.RawMessage(`{"count":2}`)); err != nil {
		t.Fatalf("valid output error = %v", err)
	}
	if err := usecase.ValidateToolOutput(tool, json.RawMessage(`{"count":"two"}`)); !errors.Is(err, domain.ErrInvalidToolOutput) {
		t.Fatalf("invalid output error = %v", err)
	}
	if err := usecase.ValidateToolOutput(tool, nil); !errors.Is(err, domain.ErrInvalidToolOutput) {
		t.Fatalf("missing output error = %v", err)
	}
}

func TestToolSchemaValidationRejectsExternalRefs(t *testing.T) {
	inputTool := domain.Tool{Name: "remote-input", InputSchema: map[string]any{"$ref": "https://example.com/schema.json"}}
	if err := usecase.ValidateToolInput(inputTool, json.RawMessage(`{}`)); !errors.Is(err, domain.ErrInvalidToolInput) {
		t.Fatalf("external input ref error = %v", err)
	}
	outputTool := domain.Tool{Name: "remote-output", OutputSchema: map[string]any{"$ref": "https://example.com/schema.json"}}
	if err := usecase.ValidateToolOutput(outputTool, json.RawMessage(`{}`)); !errors.Is(err, domain.ErrInvalidToolOutput) {
		t.Fatalf("external output ref error = %v", err)
	}
}

func TestCompileToolSchemaRejectsMalformedSchema(t *testing.T) {
	tool := domain.Tool{Name: "broken", InputSchema: map[string]any{"type": "object", "required": "path"}}
	if _, err := usecase.CompileToolInputValidator(tool); !errors.Is(err, domain.ErrInvalidToolInput) {
		t.Fatalf("CompileToolInputValidator() error = %v, want ErrInvalidToolInput", err)
	}
}
