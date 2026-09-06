package model

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"time"
)

type accumulatedToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

type openAIStream struct {
	reader           *bufio.Reader
	closer           io.Closer
	toolCalls        map[int]*accumulatedToolCall
	respToolCalls    map[string]*accumulatedToolCall
	queue            []Event
	done             bool
	terminalErr      error
	startedAt        time.Time
	linesRead        int
	bytesRead        int
	dataLines        int
	ignoredLines     int
	generatedCallSeq uint64
}

func newOpenAIStream(r io.ReadCloser) *openAIStream {
	return &openAIStream{
		reader:        bufio.NewReader(r),
		closer:        r,
		toolCalls:     make(map[int]*accumulatedToolCall),
		respToolCalls: make(map[string]*accumulatedToolCall),
		queue:         make([]Event, 0),
		startedAt:     time.Now(),
	}
}

type openAIChunk struct {
	Choices []struct {
		Delta struct {
			Role      string `json:"role,omitempty"`
			Content   string `json:"content,omitempty"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id,omitempty"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function,omitempty"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (s *openAIStream) Next(ctx context.Context) (Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			slog.DebugContext(ctx, "model stream cancelled",
				"error_type", fmt.Sprintf("%T", err),
				"duration_ms", time.Since(s.startedAt).Milliseconds(),
			)
			return Event{}, err
		}

		if len(s.queue) > 0 {
			ev := s.queue[0]
			s.queue = s.queue[1:]
			return ev, nil
		}

		if s.terminalErr != nil {
			return Event{}, s.terminalErr
		}

		if s.done {
			return Event{}, io.EOF
		}

		line, err := s.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				slog.DebugContext(ctx, "model stream reached EOF",
					"partial_line_bytes", len(line),
					"has_partial_data", len(strings.TrimSpace(line)) > 0,
				)
				if len(strings.TrimSpace(line)) > 0 {
					if parseErr := s.processLine(line); parseErr != nil {
						slog.DebugContext(ctx, "model stream parse failed",
							"error_type", fmt.Sprintf("%T", parseErr),
						)
						return Event{}, parseErr
					}
				}
				if !s.done {
					s.done = true
					s.terminalErr = fmt.Errorf("%w: provider closed before terminal event", ErrIncompleteStream)
					slog.DebugContext(ctx, "model stream incomplete",
						s.streamArgs(
							"reason", "eof_before_terminal_event",
							"duration_ms", time.Since(s.startedAt).Milliseconds(),
						)...,
					)
				}
				if len(s.queue) > 0 {
					ev := s.queue[0]
					s.queue = s.queue[1:]
					return ev, nil
				}
				if s.terminalErr != nil {
					return Event{}, s.terminalErr
				}
				return Event{}, io.EOF
			}
			slog.DebugContext(ctx, "model stream read failed",
				"error_type", fmt.Sprintf("%T", err),
				"duration_ms", time.Since(s.startedAt).Milliseconds(),
			)
			return Event{}, fmt.Errorf("read stream: %w", err)
		}

		if parseErr := s.processLine(line); parseErr != nil {
			slog.DebugContext(ctx, "model stream parse failed",
				"error_type", fmt.Sprintf("%T", parseErr),
			)
			return Event{}, parseErr
		}

		if len(s.queue) > 0 {
			ev := s.queue[0]
			s.queue = s.queue[1:]
			return ev, nil
		}
	}
}

func (s *openAIStream) processLine(line string) error {
	s.linesRead++
	s.bytesRead += len(line)
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") {
		s.ignoredLines++
		return nil
	}

	if !strings.HasPrefix(line, "data:") {
		s.ignoredLines++
		return nil
	}
	s.dataLines++

	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if payload == "[DONE]" {
		s.finish("sse_done")
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return fmt.Errorf("%w: decode SSE data: %w", ErrInvalidEvent, err)
	}

	if _, hasChoices := fields["choices"]; hasChoices {
		var chunk openAIChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("%w: decode chat completion event: %w", ErrInvalidEvent, err)
		}
		if chunk.Error != nil {
			slog.Debug("model stream provider error", "format", "chat_completions")
			return fmt.Errorf("model error: %s", chunk.Error.Message)
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				s.queue = append(s.queue, Event{
					Kind: EventTextDelta,
					Text: choice.Delta.Content,
				})
			}

			for _, tc := range choice.Delta.ToolCalls {
				acc, exists := s.toolCalls[tc.Index]
				if !exists {
					acc = &accumulatedToolCall{}
					s.toolCalls[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.arguments.WriteString(tc.Function.Arguments)
				}
			}
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != nil {
				s.finish("chat_finish_reason")
				break
			}
		}
		return nil
	}
	if _, hasError := fields["error"]; hasError {
		var chunk openAIChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("%w: decode provider error event: %w", ErrInvalidEvent, err)
		}
		if chunk.Error != nil {
			slog.Debug("model stream provider error", "format", "chat_completions")
			return fmt.Errorf("model error: %s", chunk.Error.Message)
		}
	}

	// Check OpenAI Responses API SSE chunk
	var respChunk openAIResponsesChunk
	if _, hasType := fields["type"]; hasType {
		if err := json.Unmarshal([]byte(payload), &respChunk); err != nil {
			return fmt.Errorf("%w: decode responses event: %w", ErrInvalidEvent, err)
		}
		if respChunk.Error != nil {
			slog.Debug("model stream provider error", "format", "responses")
			return fmt.Errorf("model error: %s", respChunk.Error.Message)
		}
		if respChunk.Response != nil && respChunk.Response.Error != nil {
			slog.Debug("model stream provider error", "format", "responses")
			return fmt.Errorf("model error: %s", respChunk.Response.Error.Message)
		}

		switch respChunk.Type {
		case "response.output_text.delta":
			if respChunk.Delta != "" {
				s.queue = append(s.queue, Event{
					Kind: EventTextDelta,
					Text: respChunk.Delta,
				})
			}
		case "response.output_item.added":
			if respChunk.Item != nil && respChunk.Item.Type == "function_call" {
				callID := respChunk.Item.CallID
				if callID == "" {
					callID = respChunk.Item.ID
				}
				s.respToolCalls[respChunk.Item.ID] = &accumulatedToolCall{
					id:   callID,
					name: respChunk.Item.Name,
				}
			}
		case "response.function_call_arguments.delta":
			if respChunk.ItemID != "" && respChunk.Delta != "" {
				if acc, exists := s.respToolCalls[respChunk.ItemID]; exists {
					acc.arguments.WriteString(respChunk.Delta)
				}
			}
		case "response.output_item.done":
			if respChunk.Item != nil && respChunk.Item.Type == "function_call" {
				callID := respChunk.Item.CallID
				if callID == "" {
					callID = respChunk.Item.ID
				}
				name := respChunk.Item.Name
				argsStr := strings.TrimSpace(respChunk.Item.Arguments)
				if acc, exists := s.respToolCalls[respChunk.Item.ID]; exists {
					if name == "" {
						name = acc.name
					}
					if callID == "" {
						callID = acc.id
					}
					if argsStr == "" {
						argsStr = strings.TrimSpace(acc.arguments.String())
					}
					delete(s.respToolCalls, respChunk.Item.ID)
				}
				if callID == "" {
					callID = s.nextGeneratedCallID()
				}
				if argsStr == "" {
					argsStr = "{}"
				}
				s.queue = append(s.queue, Event{
					Kind: EventToolCall,
					ToolCall: ToolCall{
						ID:        callID,
						Name:      name,
						Arguments: json.RawMessage(argsStr),
					},
				})
			}
		case "response.completed":
			s.finish("responses_completed")
		}
		return nil
	}
	return fmt.Errorf("%w: unsupported SSE data payload", ErrInvalidEvent)
}

func (s *openAIStream) finish(reason string) {
	if s.done {
		return
	}
	s.flushToolCalls()
	s.queue = append(s.queue, Event{Kind: EventDone})
	s.done = true
	slog.Debug("model stream completed", s.streamArgs(
		"reason", reason,
		"duration_ms", time.Since(s.startedAt).Milliseconds(),
	)...)
}

func (s *openAIStream) streamAttrs() []any {
	return []any{
		"lines_read", s.linesRead,
		"bytes_read", s.bytesRead,
		"data_lines", s.dataLines,
		"ignored_lines", s.ignoredLines,
	}
}

func (s *openAIStream) streamArgs(args ...any) []any {
	return append(args, s.streamAttrs()...)
}

func (s *openAIStream) nextGeneratedCallID() string {
	s.generatedCallSeq++
	return fmt.Sprintf("generated_call_%d", s.generatedCallSeq)
}

func (s *openAIStream) flushToolCalls() {
	if len(s.toolCalls) > 0 {
		indices := make([]int, 0, len(s.toolCalls))
		for idx := range s.toolCalls {
			indices = append(indices, idx)
		}
		sort.Ints(indices)

		for _, idx := range indices {
			acc := s.toolCalls[idx]
			argsStr := strings.TrimSpace(acc.arguments.String())
			if argsStr == "" {
				argsStr = "{}"
			}
			callID := acc.id
			if callID == "" {
				callID = s.nextGeneratedCallID()
			}
			s.queue = append(s.queue, Event{
				Kind: EventToolCall,
				ToolCall: ToolCall{
					ID:        callID,
					Name:      acc.name,
					Arguments: json.RawMessage(argsStr),
				},
			})
		}
		s.toolCalls = make(map[int]*accumulatedToolCall)
	}

	if len(s.respToolCalls) > 0 {
		itemIDs := make([]string, 0, len(s.respToolCalls))
		for itemID := range s.respToolCalls {
			itemIDs = append(itemIDs, itemID)
		}
		sort.Strings(itemIDs)
		for _, itemID := range itemIDs {
			acc := s.respToolCalls[itemID]
			argsStr := strings.TrimSpace(acc.arguments.String())
			if argsStr == "" {
				argsStr = "{}"
			}
			callID := acc.id
			if callID == "" {
				callID = itemID
			}
			if callID == "" {
				callID = s.nextGeneratedCallID()
			}
			s.queue = append(s.queue, Event{
				Kind: EventToolCall,
				ToolCall: ToolCall{
					ID:        callID,
					Name:      acc.name,
					Arguments: json.RawMessage(argsStr),
				},
			})
		}
		s.respToolCalls = make(map[string]*accumulatedToolCall)
	}
}

func (s *openAIStream) Close() error {
	if s.closer != nil {
		err := s.closer.Close()
		slog.Debug("model stream closed",
			s.streamArgs(
				"duration_ms", time.Since(s.startedAt).Milliseconds(),
				"close_error", err != nil,
			)...,
		)
		return err
	}
	slog.Debug("model stream closed", s.streamArgs(
		"duration_ms", time.Since(s.startedAt).Milliseconds(),
	)...)
	return nil
}
