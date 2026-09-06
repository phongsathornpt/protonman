package protonsdk

import "context"

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

func (m *middlewareModel) Provider() string { return m.base.Provider() }
func (m *middlewareModel) ModelID() string  { return m.base.ModelID() }
func (m *middlewareModel) Stream(ctx context.Context, request Request) (Stream, error) {
	return m.stream(ctx, request)
}
