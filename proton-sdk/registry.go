package protonsdk

import (
	"context"
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

// StreamFunc is the middleware-callable form of LanguageModel.Stream.
type StreamFunc func(ctx context.Context, request Request) (Stream, error)

// Middleware wraps language-model streaming without depending on provider wire formats.
type Middleware interface {
	WrapStream(next StreamFunc) StreamFunc
}

// MiddlewareFunc adapts a function to Middleware.
type MiddlewareFunc func(next StreamFunc) StreamFunc

func (f MiddlewareFunc) WrapStream(next StreamFunc) StreamFunc { return f(next) }

// WrapLanguageModel applies middleware in declaration order, with the first
// middleware acting as the outermost wrapper.
func WrapLanguageModel(model LanguageModel, middleware ...Middleware) LanguageModel {
	if model == nil || len(middleware) == 0 {
		return model
	}
	next := StreamFunc(model.Stream)
	for i := len(middleware) - 1; i >= 0; i-- {
		if middleware[i] != nil {
			next = middleware[i].WrapStream(next)
		}
	}
	return &middlewareModel{base: model, stream: next}
}

type middlewareModel struct {
	base   LanguageModel
	stream StreamFunc
}

func (m *middlewareModel) Provider() string                { return m.base.Provider() }
func (m *middlewareModel) ModelID() string                 { return m.base.ModelID() }
func (m *middlewareModel) Capabilities() ModelCapabilities { return m.base.Capabilities() }
func (m *middlewareModel) Metadata() ModelMetadata         { return ModelMetadataOf(m.base) }
func (m *middlewareModel) TokenLimits() TokenLimits        { return ModelTokenLimits(m.base) }
func (m *middlewareModel) ContextWindow() int              { return ModelContextWindow(m.base) }
func (m *middlewareModel) Stream(ctx context.Context, request Request) (Stream, error) {
	return m.stream(ctx, request)
}
