package protonsdk

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateToolOutput(t *testing.T) {
	tool := Tool{Name: "structured", OutputSchema: map[string]any{
		"type":       "object",
		"properties": map[string]any{"count": map[string]any{"type": "number"}},
		"required":   []any{"count"},
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

func TestValidateToolOutputRejectsExternalRefs(t *testing.T) {
	tool := Tool{Name: "remote", OutputSchema: map[string]any{"$ref": "https://example.com/schema.json"}}
	if err := ValidateToolOutput(tool, json.RawMessage(`{}`)); !errors.Is(err, ErrInvalidToolOutput) {
		t.Fatalf("external ref error = %v", err)
	}
}
