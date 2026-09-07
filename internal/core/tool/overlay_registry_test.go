package tool

import (
	"context"
	"encoding/json"
	"testing"
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
