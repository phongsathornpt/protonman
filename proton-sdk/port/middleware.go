package port

import (
	"context"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

// ModelFactory resolves one provider-local model identifier.
type ModelFactory func(modelID string) LanguageModel

// StreamFunc is the middleware-callable form of LanguageModel.Stream.
type StreamFunc func(ctx context.Context, request domain.Request) (Stream, error)

// Middleware wraps language-model streaming without depending on provider wire formats.
type Middleware interface {
	WrapStream(next StreamFunc) StreamFunc
}

// MiddlewareFunc adapts a function to Middleware.
type MiddlewareFunc func(next StreamFunc) StreamFunc

func (f MiddlewareFunc) WrapStream(next StreamFunc) StreamFunc { return f(next) }
