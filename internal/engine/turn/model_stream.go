package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/adapter/out/model"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func (l *Loop) streamRound(
	ctx context.Context,
	round int,
	request sdk.Request,
	sink Sink,
) (model.Message, []model.ToolCall, error) {
	startedAt := time.Now()
	slog.DebugContext(ctx, "model round stream opening",
		"round", round,
		"message_count", len(request.Messages),
		"tool_count", len(request.Tools),
	)
	stream, err := l.languageModel.Stream(ctx, request)
	if err != nil {
		slog.DebugContext(ctx, "model round stream open failed",
			"round", round,
			"error_type", fmt.Sprintf("%T", err),
		)
		return model.Message{}, nil, fmt.Errorf("stream model round %d: %w", round, err)
	}
	if stream == nil {
		return model.Message{}, nil, fmt.Errorf("stream model round %d: nil stream", round)
	}
	defer func() {
		closeErr := stream.Close()
		slog.DebugContext(ctx, "model round stream closed",
			"round", round,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"close_error", closeErr != nil,
		)
	}()
	assistant, calls, streamErr := consumeSDKStream(ctx, round, stream, sink)
	if streamErr != nil {
		return model.Message{}, nil, streamErr
	}
	slog.DebugContext(ctx, "model round stream consumed",
		"round", round,
		"assistant_bytes", len(assistant.Content),
		"tool_calls", len(calls),
	)
	return assistant, calls, nil
}

func consumeSDKStream(ctx context.Context, round int, stream sdk.Stream, sink Sink) (model.Message, []model.ToolCall, error) {
	var text strings.Builder
	calls := make([]model.ToolCall, 0)
	for {
		event, err := stream.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return model.Message{}, nil, fmt.Errorf("read model stream round %d: %w", round, err)
		}
		if err := event.Validate(); err != nil {
			return model.Message{}, nil, fmt.Errorf("validate model stream round %d: %w", round, err)
		}
		switch event.Kind {
		case sdk.EventTextDelta:
			text.WriteString(event.Text)
			if err := emit(ctx, sink, Event{Kind: EventTextDelta, Round: round, Text: event.Text}); err != nil {
				return model.Message{}, nil, err
			}
		case sdk.EventToolCall:
			call := event.ToolCall
			call.Arguments = append(json.RawMessage(nil), call.Arguments...)
			calls = append(calls, call)
		case sdk.EventFinish:
			if text.Len() == 0 && len(calls) == 0 {
				return model.Message{}, nil, fmt.Errorf("model stream round %d: %w", round, ErrEmptyResponse)
			}
			return model.Message{Role: model.RoleAssistant, Content: text.String(), ToolCalls: calls}, calls, nil
		}
	}
	return model.Message{}, nil, fmt.Errorf("read model stream round %d: %w", round, sdk.ErrIncompleteStream)
}
