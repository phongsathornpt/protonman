package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
)

// ResponseAccumulator incrementally reconstructs one canonical Response from
// normalized stream events while keeping stream transport concerns separate.
type ResponseAccumulator struct {
	text     strings.Builder
	response domain.Response
	finished bool
}

// Absorb applies one normalized stream event to the response state machine.
// A terminal finish event seals the accumulator; later events are rejected.
func (a *ResponseAccumulator) Absorb(event domain.Event) error {
	if a == nil {
		return fmt.Errorf("%w: response accumulator is required", domain.ErrInvalidEvent)
	}
	if a.finished {
		return fmt.Errorf("%w: response already finished", domain.ErrInvalidEvent)
	}
	if err := event.Validate(); err != nil {
		return err
	}

	switch event.Kind {
	case domain.EventTextDelta:
		a.text.WriteString(event.Text)
	case domain.EventReasoningDelta:
		a.response.ReasoningContent += event.ReasoningContent
	case domain.EventToolCall:
		a.response.ToolCalls = append(a.response.ToolCalls, event.ToolCall.Clone())
	case domain.EventUsage:
		a.response.Usage = event.Usage
	case domain.EventFinish:
		a.response.FinishReason = event.FinishReason
		a.response.ProviderMetadata = event.ProviderMetadata.Clone()
		a.finished = true
	}
	return nil
}

// Finish returns the completed canonical response. Calling Finish before a
// terminal event preserves the SDK's incomplete-stream invariant.
func (a *ResponseAccumulator) Finish() (domain.Response, error) {
	if a == nil || !a.finished {
		return domain.Response{}, domain.ErrIncompleteStream
	}
	response := a.response
	response.Text = a.text.String()
	response.ProviderMetadata = a.response.ProviderMetadata.Clone()
	response.ToolCalls = make([]domain.ToolCall, 0, len(a.response.ToolCalls))
	for _, call := range a.response.ToolCalls {
		response.ToolCalls = append(response.ToolCalls, call.Clone())
	}
	return response, nil
}

// Collect consumes one model stream until its terminal event and builds a
// provider-neutral response through the canonical response accumulator.
func Collect(ctx context.Context, stream port.Stream) (result domain.Response, err error) {
	if stream == nil {
		return domain.Response{}, fmt.Errorf("%w: stream is required", domain.ErrInvalidRequest)
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
				return domain.Response{}, domain.ErrIncompleteStream
			}
			return domain.Response{}, err
		}
		if err := accumulator.Absorb(event); err != nil {
			return domain.Response{}, err
		}
		if event.Kind == domain.EventFinish {
			return accumulator.Finish()
		}
	}
}

// CollectStep is retained for source compatibility with earlier SDK releases.
// Deprecated: use Collect.
func CollectStep(ctx context.Context, stream port.Stream) (domain.StepResult, error) {
	return Collect(ctx, stream)
}
