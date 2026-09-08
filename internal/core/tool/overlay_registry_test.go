package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	sdk "github.com/phongsathornpt/proton/proton-sdk"
)

type overlayTestHandler struct{ name, description string }

func (h overlayTestHandler) Definition() Definition {
	return Definition{Name: h.name, Description: h.description, Kind: KindRead, Mutability: MutabilityReadOnly, Safety: SafetyContract{MutationDomain: MutationDomainNone, MutationSafety: MutationSafetyNone, CheckpointPolicy: CheckpointPolicyNone, Boundary: BoundaryPolicyNone}, InputSchema: NoArgumentsSchema()}
}
func (h overlayTestHandler) Execute(context.Context, Call) (Result, error) { return Result{}, nil }

type overlayTestRegistry struct{ handlers []Handler }

func (r overlayTestRegistry) Lookup(name string) (Handler, bool) {
	for _, h := range r.handlers {
		if h.Definition().Name == name {
			return h, true
		}
	}
	return nil, false
}
func (r overlayTestRegistry) Definitions() []Definition {
	out := make([]Definition, 0, len(r.handlers))
	for _, h := range r.handlers {
		out = append(out, h.Definition())
	}
	return out
}

func TestOverlayRegistryReplacesHandlerAndPreservesOrder(t *testing.T) {
	_ = json.RawMessage(nil)
	base := overlayTestRegistry{handlers: []Handler{overlayTestHandler{"a", "base a"}, overlayTestHandler{"b", "base b"}}}
	r, err := NewOverlayRegistry(base, overlayTestHandler{"b", "session b"})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := r.Lookup("b")
	if !ok || h.Definition().Description != "session b" {
		t.Fatalf("lookup=%v %v", h, ok)
	}
	defs := r.Definitions()
	if len(defs) != 2 || defs[0].Name != "a" || defs[1].Description != "session b" {
		t.Fatalf("defs=%+v", defs)
	}
}

type overlayDynamicRegistry struct{ overlayTestRegistry }

func (r *overlayDynamicRegistry) Register(handler Handler) error {
	r.handlers = append(r.handlers, handler)
	return nil
}
func (r *overlayDynamicRegistry) RegisterBatch(handlers []Handler) error {
	for _, handler := range handlers {
		if err := r.Register(handler); err != nil {
			return err
		}
	}
	return nil
}
func (r *overlayDynamicRegistry) ReplaceNamespace(prefix string, handlers []Handler) error {
	kept := r.handlers[:0]
	for _, handler := range r.handlers {
		if !strings.HasPrefix(handler.Definition().Name, prefix) {
			kept = append(kept, handler)
		}
	}
	r.handlers = kept
	return r.RegisterBatch(handlers)
}

func TestOverlayRegistryPreservesDynamicRegistrar(t *testing.T) {
	base := &overlayDynamicRegistry{overlayTestRegistry{handlers: []Handler{
		overlayTestHandler{"a", "base a"},
		overlayTestHandler{"b", "base b"},
	}}}
	reg, err := NewOverlayRegistry(base, overlayTestHandler{"b", "session b"})
	if err != nil {
		t.Fatal(err)
	}
	dynamic, ok := reg.(DynamicRegistrar)
	if !ok {
		t.Fatal("overlay registry dropped DynamicRegistrar")
	}
	if err := dynamic.Register(overlayTestHandler{"mcp.test.echo", "echo"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := reg.Lookup("mcp.test.echo"); !ok {
		t.Fatal("dynamically registered handler is not visible through overlay")
	}
}

type overlayCompiledRegistry struct{ overlayTestRegistry }

func (r overlayCompiledRegistry) CompiledValidators(name string) (*sdk.ToolSchemaValidator, *sdk.ToolSchemaValidator, bool) {
	_, ok := r.Lookup(name)
	return nil, nil, ok
}

func TestOverlayRegistryPreservesCompiledValidatorsExceptOverrides(t *testing.T) {
	base := overlayCompiledRegistry{overlayTestRegistry{handlers: []Handler{
		overlayTestHandler{"a", "base a"},
		overlayTestHandler{"b", "base b"},
	}}}
	reg, err := NewOverlayRegistry(base, overlayTestHandler{"b", "session b"})
	if err != nil {
		t.Fatal(err)
	}
	compiled, ok := reg.(interface {
		CompiledValidators(string) (*sdk.ToolSchemaValidator, *sdk.ToolSchemaValidator, bool)
	})
	if !ok {
		t.Fatal("overlay registry dropped compiled validator cache")
	}
	if _, _, found := compiled.CompiledValidators("a"); !found {
		t.Fatal("base validator cache was not forwarded")
	}
	if _, _, found := compiled.CompiledValidators("b"); found {
		t.Fatal("overlay reused stale base validators for overridden schema")
	}
}
