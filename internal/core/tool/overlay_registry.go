package tool

import (
	"fmt"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// OverlayRegistry replaces selected handlers while preserving the base
// registry's definition order. It is used for session-bound stateful tools.
type OverlayRegistry struct {
	base      Registry
	overrides map[string]Handler
}

func NewOverlayRegistry(base Registry, overrides ...Handler) (Registry, error) {
	if base == nil {
		return nil, fmt.Errorf("overlay registry base is required")
	}
	r := &OverlayRegistry{base: base, overrides: make(map[string]Handler, len(overrides))}
	for _, handler := range overrides {
		if handler == nil {
			return nil, fmt.Errorf("overlay registry handler is required")
		}
		def := handler.Definition()
		if err := def.Validate(); err != nil {
			return nil, err
		}
		if _, exists := r.overrides[def.Name]; exists {
			return nil, fmt.Errorf("duplicate overlay tool %q", def.Name)
		}
		if _, exists := base.Lookup(def.Name); !exists {
			return nil, fmt.Errorf("overlay tool %q is not registered in base registry", def.Name)
		}
		r.overrides[def.Name] = handler
	}
	if dynamic, ok := base.(DynamicRegistrar); ok {
		return &dynamicOverlayRegistry{OverlayRegistry: r, dynamic: dynamic}, nil
	}
	return r, nil
}

type dynamicOverlayRegistry struct {
	*OverlayRegistry
	dynamic DynamicRegistrar
}

func (r *dynamicOverlayRegistry) Register(handler Handler) error {
	return r.dynamic.Register(handler)
}

func (r *dynamicOverlayRegistry) RegisterBatch(handlers []Handler) error {
	return r.dynamic.RegisterBatch(handlers)
}

func (r *dynamicOverlayRegistry) ReplaceNamespace(prefix string, handlers []Handler) error {
	return r.dynamic.ReplaceNamespace(prefix, handlers)
}

func (r *OverlayRegistry) Lookup(name string) (Handler, bool) {
	if r == nil {
		return nil, false
	}
	if handler, ok := r.overrides[name]; ok {
		return handler, true
	}
	return r.base.Lookup(name)
}

func (r *OverlayRegistry) CompiledValidators(name string) (input, output *sdk.ToolSchemaValidator, ok bool) {
	if r == nil || r.base == nil {
		return nil, nil, false
	}
	if _, overridden := r.overrides[name]; overridden {
		return nil, nil, false
	}
	type compiledRegistry interface {
		CompiledValidators(string) (*sdk.ToolSchemaValidator, *sdk.ToolSchemaValidator, bool)
	}
	compiled, ok := r.base.(compiledRegistry)
	if !ok {
		return nil, nil, false
	}
	return compiled.CompiledValidators(name)
}

func (r *OverlayRegistry) Definitions() []Definition {
	if r == nil || r.base == nil {
		return nil
	}
	defs := r.base.Definitions()
	for i := range defs {
		if handler, ok := r.overrides[defs[i].Name]; ok {
			defs[i] = handler.Definition()
		}
	}
	return defs
}
