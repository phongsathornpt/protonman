package protonsdk

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateToolInput(t *testing.T) {
	tool := Tool{Name: "read_file", InputSchema: map[string]any{
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
