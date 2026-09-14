package usecase

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
)

// Registry resolves scoped model IDs such as "anthropic/claude-sonnet-5".
type Registry struct {
	mu        sync.RWMutex
	providers map[string]port.ModelFactory
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]port.ModelFactory)}
}

func (r *Registry) Register(provider string, factory port.ModelFactory) error {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return fmt.Errorf("%w: provider name is required", domain.ErrInvalidRequest)
	}
	if factory == nil {
		return fmt.Errorf("%w: model factory is required for %q", domain.ErrInvalidRequest, provider)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[provider] = factory
	return nil
}

func (r *Registry) Model(scopedID string) (port.LanguageModel, error) {
	provider, modelID, ok := strings.Cut(strings.TrimSpace(scopedID), "/")
	provider = strings.ToLower(strings.TrimSpace(provider))
	modelID = strings.TrimSpace(modelID)
	if !ok || provider == "" || modelID == "" {
		return nil, fmt.Errorf("%w: model id %q must use provider/model format", domain.ErrInvalidRequest, scopedID)
	}
	r.mu.RLock()
	factory := r.providers[provider]
	r.mu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("%w: provider %q is not registered", domain.ErrInvalidRequest, provider)
	}
	model := factory(modelID)
	if model == nil {
		return nil, fmt.Errorf("%w: provider %q returned no model for %q", domain.ErrInvalidRequest, provider, modelID)
	}
	return model, nil
}

// WrapLanguageModel applies middleware in declaration order, with the first
// middleware acting as the outermost wrapper.
func WrapLanguageModel(model port.LanguageModel, middleware ...port.Middleware) port.LanguageModel {
	if model == nil || len(middleware) == 0 {
		return model
	}
	next := port.StreamFunc(model.Stream)
	for i := len(middleware) - 1; i >= 0; i-- {
		if middleware[i] != nil {
			next = middleware[i].WrapStream(next)
		}
	}
	return &middlewareModel{base: model, stream: next}
}

type middlewareModel struct {
	base   port.LanguageModel
	stream port.StreamFunc
}

func (m *middlewareModel) Provider() string                     { return m.base.Provider() }
func (m *middlewareModel) ModelID() string                      { return m.base.ModelID() }
func (m *middlewareModel) Capabilities() domain.ModelCapabilities { return m.base.Capabilities() }
func (m *middlewareModel) Metadata() domain.ModelMetadata       { return ModelMetadataOf(m.base) }
func (m *middlewareModel) TokenLimits() domain.TokenLimits      { return ModelTokenLimits(m.base) }
func (m *middlewareModel) ContextWindow() int                   { return ModelContextWindow(m.base) }
func (m *middlewareModel) Stream(ctx context.Context, request domain.Request) (port.Stream, error) {
	return m.stream(ctx, request)
}
