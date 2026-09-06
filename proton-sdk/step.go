package protonsdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// StepResult is the normalized output of one model step in an agent loop.
type StepResult struct {
	Text             string
	ToolCalls        []ToolCall
	Usage            Usage
	FinishReason     FinishReason
	ProviderMetadata ProviderMetadata
}

// CollectStep consumes one model stream until its terminal event and builds the
// provider-neutral result an agent loop needs for its next decision.
func CollectStep(ctx context.Context, stream Stream) (StepResult, error) {
	if stream == nil {
		return StepResult{}, fmt.Errorf("%w: stream is required", ErrInvalidRequest)
	}
	var result StepResult
	var text strings.Builder
	for {
		event, err := stream.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return StepResult{}, ErrIncompleteStream
			}
			return StepResult{}, err
		}
		if err := event.Validate(); err != nil {
			return StepResult{}, err
		}
		switch event.Kind {
		case EventTextDelta:
			text.WriteString(event.Text)
		case EventToolCall:
			result.ToolCalls = append(result.ToolCalls, cloneToolCall(event.ToolCall))
		case EventUsage:
			result.Usage = event.Usage
		case EventFinish:
			result.Text = text.String()
			result.ProviderMetadata = cloneProviderMetadata(event.ProviderMetadata)
			result.FinishReason = event.FinishReason
			return result, nil
		}
	}
}

func cloneToolCall(call ToolCall) ToolCall {
	call.Arguments = append([]byte(nil), call.Arguments...)
	return call
}

func cloneProviderMetadata(metadata ProviderMetadata) ProviderMetadata {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(ProviderMetadata, len(metadata))
	for key, value := range metadata {
		cloned[key] = append(json.RawMessage(nil), value...)
	}
	return cloned
}
