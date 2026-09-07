package builtin

import (
	"context"
	"testing"

	"github.com/projectTHORN/proton/internal/tool"
)

type invalidSchemaHandler struct{}

func (invalidSchemaHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        "invalid_schema",
		Description: "invalid schema fixture",
		Kind:        tool.KindRead,
		Mutability:  tool.MutabilityReadOnly,
		InputSchema: map[string]any{
			"type":     "object",
			"required": "path",
		},
	}
}

func (invalidSchemaHandler) Execute(context.Context, tool.Call) (tool.Result, error) {
	return tool.Result{}, nil
}

func TestRegistryRejectsMalformedToolSchema(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(invalidSchemaHandler{}); err == nil {
		t.Fatal("Register() error = nil, want malformed schema rejection")
	}
	if got := len(registry.Definitions()); got != 0 {
		t.Fatalf("definitions = %d after rejected schema, want 0", got)
	}
}
