package tool

import "fmt"

// OverlayRegistry replaces selected handlers while preserving the base
// registry's definition order. It is used for session-bound stateful tools.
type OverlayRegistry struct {
	base      Registry
	overrides map[string]Handler
}

func NewOverlayRegistry(base Registry, overrides ...Handler) (*OverlayRegistry, error) {
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
	return r, nil
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
