package builtin

import (
	"context"
	"testing"

	"github.com/projectTHORN/proton/internal/core/tool"
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

type cachedSchemaHandler struct{}

func (cachedSchemaHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        "cached_schema",
		Description: "cached schema fixture",
		Kind:        tool.KindRead,
		Mutability:  tool.MutabilityReadOnly,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
			},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ok": map[string]any{"type": "boolean"},
			},
		},
	}
}

func (cachedSchemaHandler) Execute(context.Context, tool.Call) (tool.Result, error) {
	return tool.Result{}, nil
}

func TestRegistryCachesCompiledSchemaValidators(t *testing.T) {
	registry, err := NewRegistry(cachedSchemaHandler{})
	if err != nil {
		t.Fatal(err)
	}
	input1, output1, ok := registry.CompiledValidators("cached_schema")
	if !ok || input1 == nil || output1 == nil {
		t.Fatalf("CompiledValidators() = (%v, %v, %v), want cached input/output validators", input1, output1, ok)
	}
	input2, output2, ok := registry.CompiledValidators("cached_schema")
	if !ok || input1 != input2 || output1 != output2 {
		t.Fatal("CompiledValidators() did not return the registration-time validator instances")
	}
	if _, _, ok := registry.CompiledValidators("missing"); ok {
		t.Fatal("CompiledValidators(missing) ok = true, want false")
	}
}

type namedSchemaHandler struct{ name string }

func (h namedSchemaHandler) Definition() tool.Definition {
	return tool.Definition{Name: h.name, Description: h.name, Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly, InputSchema: tool.NoArgumentsSchema()}
}
func (h namedSchemaHandler) Execute(context.Context, tool.Call) (tool.Result, error) {
	return tool.Result{}, nil
}

func TestRegistryRegisterBatchIsAtomic(t *testing.T) {
	registry, err := NewRegistry(namedSchemaHandler{name: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	err = registry.RegisterBatch([]tool.Handler{
		namedSchemaHandler{name: "new-one"},
		namedSchemaHandler{name: "existing"},
	})
	if err == nil {
		t.Fatal("RegisterBatch() error = nil")
	}
	if _, ok := registry.Lookup("new-one"); ok {
		t.Fatal("new-one was partially registered")
	}
	if got := len(registry.Definitions()); got != 1 {
		t.Fatalf("definitions = %d, want 1", got)
	}
}
