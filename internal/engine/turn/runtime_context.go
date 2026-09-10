package turn

import (
	"context"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

// RuntimeContextProvider supplies ephemeral, non-persistent context produced by
// asynchronous runtime capabilities such as delegated subagents. Implementations
// must treat the caller context as the current turn scope.
type RuntimeContextProvider interface {
	// Drain returns context that is already available without waiting.
	Drain(context.Context) ([]model.Message, error)
	// Await waits until meaningful runtime context becomes available or the
	// caller context ends. It should return promptly when no work is pending.
	Await(context.Context) ([]model.Message, error)
	// Pending reports whether the current turn still owns asynchronous work
	// whose result may need integration before final completion.
	Pending(context.Context) bool
}

// WithRuntimeContextProvider enables event-driven runtime context delivery.
func WithRuntimeContextProvider(provider RuntimeContextProvider) Option {
	return func(loop *Loop) error {
		loop.runtimeContext = provider
		return nil
	}
}

type runtimeEventBuffer struct {
	events []Event
}

func (b *runtimeEventBuffer) sink(_ context.Context, event Event) error {
	b.events = append(b.events, event)
	return nil
}

func (b *runtimeEventBuffer) flush(ctx context.Context, sink Sink) error {
	if b == nil {
		return nil
	}
	for _, event := range b.events {
		if err := emit(ctx, sink, event); err != nil {
			return err
		}
	}
	b.events = nil
	return nil
}

func (l *Loop) completionRuntimeContext(ctx context.Context) ([]model.Message, bool, error) {
	if l == nil || l.runtimeContext == nil {
		return nil, false, nil
	}
	messages, err := l.runtimeContext.Drain(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(messages) > 0 {
		return model.EnsureMessageIDs(messages), true, nil
	}
	if !l.runtimeContext.Pending(ctx) {
		return nil, false, nil
	}
	messages, err = l.runtimeContext.Await(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(messages) > 0 {
		return model.EnsureMessageIDs(messages), true, nil
	}
	return nil, l.runtimeContext.Pending(ctx), nil
}
