package protonsdk

import (
	"fmt"
	"strings"
)

// ResponseAccumulator incrementally reconstructs one canonical Response from
// normalized stream events while keeping stream transport concerns separate.
type ResponseAccumulator struct {
	text     strings.Builder
	response Response
	finished bool
}

// Absorb applies one normalized stream event to the response state machine.
// A terminal finish event seals the accumulator; later events are rejected.
func (a *ResponseAccumulator) Absorb(event Event) error {
	if a == nil {
		return fmt.Errorf("%w: response accumulator is required", ErrInvalidEvent)
	}
	if a.finished {
		return fmt.Errorf("%w: response already finished", ErrInvalidEvent)
	}
	if err := event.Validate(); err != nil {
		return err
	}

	switch event.Kind {
	case EventTextDelta:
		a.text.WriteString(event.Text)
	case EventToolCall:
		a.response.ToolCalls = append(a.response.ToolCalls, cloneToolCall(event.ToolCall))
	case EventUsage:
		// Usage events are normalized as complete snapshots by the current
		// provider adapters. Keep the latest snapshot until the usage model can
		// represent optional or partial counters explicitly.
		a.response.Usage = event.Usage
	case EventFinish:
		a.response.FinishReason = event.FinishReason
		a.response.ProviderMetadata = cloneProviderMetadata(event.ProviderMetadata)
		a.finished = true
	}
	return nil
}

// Finish returns the completed canonical response. Calling Finish before a
// terminal event preserves the SDK's incomplete-stream invariant.
func (a *ResponseAccumulator) Finish() (Response, error) {
	if a == nil || !a.finished {
		return Response{}, ErrIncompleteStream
	}
	response := a.response
	response.Text = a.text.String()
	response.ProviderMetadata = cloneProviderMetadata(a.response.ProviderMetadata)
	response.ToolCalls = make([]ToolCall, 0, len(a.response.ToolCalls))
	for _, call := range a.response.ToolCalls {
		response.ToolCalls = append(response.ToolCalls, cloneToolCall(call))
	}
	return response, nil
}
