package usecase_test

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

func TestToolSchemaValidation(t *testing.T) {
	tool := domain.Tool{
		Name: "test_tool",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"num": map[string]any{"type": "number"},
			},
			"required": []string{"num"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"res": map[string]any{"type": "string"},
			},
			"required": []string{"res"},
		},
	}

	// Valid input
	if err := usecase.ValidateToolInput(tool, json.RawMessage(`{"num": 42}`)); err != nil {
		t.Fatalf("valid input failed: %v", err)
	}

	// Invalid input
	if err := usecase.ValidateToolInput(tool, json.RawMessage(`{"num": "not_a_number"}`)); err == nil {
		t.Fatal("invalid input type should fail")
	}

	// Valid output
	if err := usecase.ValidateToolOutput(tool, json.RawMessage(`{"res": "ok"}`)); err != nil {
		t.Fatalf("valid output failed: %v", err)
	}

	// Invalid output
	if err := usecase.ValidateToolOutput(tool, json.RawMessage(`{"res": 123}`)); err == nil {
		t.Fatal("invalid output type should fail")
	}
}

func TestSchemaValidatorNilAndEmpty(t *testing.T) {
	toolNoSchema := domain.Tool{Name: "empty"}
	if err := usecase.ValidateToolInput(toolNoSchema, json.RawMessage(`{"anything": 1}`)); err != nil {
		t.Fatalf("empty schema validation should pass: %v", err)
	}
	if err := usecase.ValidateToolOutput(toolNoSchema, json.RawMessage(`{"anything": 1}`)); err != nil {
		t.Fatalf("empty schema validation should pass: %v", err)
	}
}
