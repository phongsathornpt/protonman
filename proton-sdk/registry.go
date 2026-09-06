package protonsdk

import (
	"fmt"
	"strings"
	"sync"
)

// ModelFactory resolves one provider-local model identifier.
type ModelFactory func(modelID string) LanguageModel

// Registry resolves scoped model IDs such as "anthropic/claude-sonnet-5".
type Registry struct {
	mu        sync.RWMutex
	providers map[string]ModelFactory
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]ModelFactory)}
}

func (r *Registry) Register(provider string, factory ModelFactory) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return fmt.Errorf("%w: provider name is required", ErrInvalidRequest)
	}
	if factory == nil {
		return fmt.Errorf("%w: model factory is required for %q", ErrInvalidRequest, provider)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider] = factory
	return nil
}

func (r *Registry) Model(scopedID string) (LanguageModel, error) {
	provider, modelID, ok := strings.Cut(strings.TrimSpace(scopedID), "/")
	provider = strings.ToLower(strings.TrimSpace(provider))
	modelID = strings.TrimSpace(modelID)
	if !ok || provider == "" || modelID == "" {
		return nil, fmt.Errorf("%w: model id %q must use provider/model format", ErrInvalidRequest, scopedID)
	}
	r.mu.RLock()
	factory := r.providers[provider]
	r.mu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("%w: provider %q is not registered", ErrInvalidRequest, provider)
	}
	model := factory(modelID)
	if model == nil {
		return nil, fmt.Errorf("%w: provider %q returned no model for %q", ErrInvalidRequest, provider, modelID)
	}
	return model, nil
}
