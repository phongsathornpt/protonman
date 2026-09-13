package protonsdk

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Collect consumes one model stream until its terminal event and builds a
// provider-neutral response through the canonical response accumulator.
func Collect(ctx context.Context, stream Stream) (result Response, err error) {
	if stream == nil {
		return Response{}, fmt.Errorf("%w: stream is required", ErrInvalidRequest)
	}
	defer func() {
		if closeErr := stream.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close model stream: %w", closeErr)
		}
	}()

	var accumulator ResponseAccumulator
	for {
		event, err := stream.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return Response{}, ErrIncompleteStream
			}
			return Response{}, err
		}
		if err := accumulator.Absorb(event); err != nil {
			return Response{}, err
		}
		if event.Kind == EventFinish {
			return accumulator.Finish()
		}
	}
}

// CollectStep is retained for source compatibility with earlier SDK releases.
// Deprecated: use Collect.
func CollectStep(ctx context.Context, stream Stream) (StepResult, error) {
	return Collect(ctx, stream)
}

func cloneToolCall(call ToolCall) ToolCall {
	call.Arguments = append([]byte(nil), call.Arguments...)
	return call
}
