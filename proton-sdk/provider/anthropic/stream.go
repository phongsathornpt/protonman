package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

type toolAccumulator struct {
	id   string
	name string
	args strings.Builder
}

type stream struct {
	reader     *bufio.Reader
	closer     io.Closer
	queue      []domain.Event
	tools      map[int]*toolAccumulator
	usage      domain.Usage
	finish     domain.FinishReason
	done       bool
	terminal   error
	metadata   domain.ProviderMetadata
	includeRaw bool
	closeOnce  sync.Once
	closeErr   error
}

func newStream(body io.ReadCloser, metadata domain.ProviderMetadata, includeRaw bool) *stream {
	return &stream{reader: bufio.NewReader(body), closer: body, tools: make(map[int]*toolAccumulator), metadata: metadata, includeRaw: includeRaw}
}

func (s *stream) Next(ctx context.Context) (domain.Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return domain.Event{}, err
		}
		if len(s.queue) > 0 {
			event := s.queue[0]
			s.queue = s.queue[1:]
			return event, nil
		}
		if s.terminal != nil {
			return domain.Event{}, s.terminal
		}
		if s.done {
			return domain.Event{}, io.EOF
		}
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if strings.TrimSpace(line) != "" {
					if parseErr := s.processLine(line); parseErr != nil {
						return domain.Event{}, parseErr
					}
				}
				if !s.done {
					s.done = true
					s.terminal = fmt.Errorf("%w: anthropic stream closed before message_stop", domain.ErrIncompleteStream)
				}
				continue
			}
			return domain.Event{}, fmt.Errorf("read anthropic stream: %w", err)
		}
		if err := s.processLine(line); err != nil {
			if len(s.queue) > 0 {
				s.terminal = err
				event := s.queue[0]
				s.queue = s.queue[1:]
				return event, nil
			}
			return domain.Event{}, err
		}
	}
}

type wireEvent struct {
	Type         string          `json:"type"`
	Index        int             `json:"index,omitempty"`
	ContentBlock json.RawMessage `json:"content_block,omitempty"`
	Delta        json.RawMessage `json:"delta,omitempty"`
	Usage        struct {
		InputTokens              int64 `json:"input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	} `json:"usage,omitempty"`
	Message *struct {
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message,omitempty"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (s *stream) processLine(line string) error {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "event:") || strings.HasPrefix(line, ":") {
		return nil
	}
	if !strings.HasPrefix(line, "data:") {
		return nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if s.includeRaw && payload != "" {
		s.queue = append(s.queue, domain.Event{Kind: domain.EventRaw, RawData: append([]byte(nil), payload...)})
	}
	var event wireEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("%w: decode anthropic event: %w", domain.ErrInvalidEvent, err)
	}
	switch event.Type {
	case "message_start":
		if event.Message != nil {
			s.usage.InputTokens = event.Message.Usage.InputTokens
			s.usage.OutputTokens = event.Message.Usage.OutputTokens
			s.usage.CachedInputTokens = event.Message.Usage.CacheReadInputTokens
			s.usage.TotalTokens = s.usage.InputTokens + s.usage.OutputTokens
			s.queue = append(s.queue, domain.Event{Kind: domain.EventUsage, Usage: s.usage})
		}
	case "content_block_start":
		var block struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Text  string          `json:"text"`
			Input json.RawMessage `json:"input"`
		}
		if err := json.Unmarshal(event.ContentBlock, &block); err != nil {
			return fmt.Errorf("%w: decode anthropic content block: %w", domain.ErrInvalidEvent, err)
		}
		switch block.Type {
		case "text":
			s.queue = append(s.queue, domain.Event{Kind: domain.EventTextStart})
			if block.Text != "" {
				s.queue = append(s.queue, domain.Event{Kind: domain.EventTextDelta, Text: block.Text})
			}
		case "tool_use":
			acc := &toolAccumulator{id: block.ID, name: block.Name}
			if len(block.Input) > 0 && string(block.Input) != "{}" {
				acc.args.Write(block.Input)
			}
			s.tools[event.Index] = acc
			s.queue = append(s.queue, domain.Event{Kind: domain.EventToolCallStart, ToolCallID: block.ID, ToolName: block.Name})
		}
	case "content_block_delta":
		var delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
		}
		if err := json.Unmarshal(event.Delta, &delta); err != nil {
			return fmt.Errorf("%w: decode anthropic content delta: %w", domain.ErrInvalidEvent, err)
		}
		switch delta.Type {
		case "text_delta":
			if delta.Text != "" {
				s.queue = append(s.queue, domain.Event{Kind: domain.EventTextDelta, Text: delta.Text})
			}
		case "input_json_delta":
			if acc := s.tools[event.Index]; acc != nil && delta.PartialJSON != "" {
				acc.args.WriteString(delta.PartialJSON)
				s.queue = append(s.queue, domain.Event{Kind: domain.EventToolCallDelta, ToolCallID: acc.id, ToolName: acc.name, ArgumentsDelta: delta.PartialJSON})
			}
		}
	case "content_block_stop":
		if acc := s.tools[event.Index]; acc != nil {
			args := strings.TrimSpace(acc.args.String())
			if args == "" {
				args = "{}"
			}
			if !json.Valid([]byte(args)) {
				return fmt.Errorf("%w: anthropic tool arguments are invalid JSON", domain.ErrInvalidEvent)
			}
			call := domain.ToolCall{ID: acc.id, Name: acc.name, Arguments: json.RawMessage(args)}
			s.queue = append(s.queue,
				domain.Event{Kind: domain.EventToolCallEnd, ToolCallID: acc.id, ToolName: acc.name},
				domain.Event{Kind: domain.EventToolCall, ToolCall: call},
			)
			delete(s.tools, event.Index)
		} else {
			s.queue = append(s.queue, domain.Event{Kind: domain.EventTextEnd})
		}
	case "message_delta":
		var delta struct {
			StopReason string `json:"stop_reason"`
		}
		_ = json.Unmarshal(event.Delta, &delta)
		s.finish = mapStopReason(delta.StopReason)
		if event.Usage.OutputTokens > 0 {
			s.usage.OutputTokens = event.Usage.OutputTokens
			s.usage.TotalTokens = s.usage.InputTokens + s.usage.OutputTokens
			s.queue = append(s.queue, domain.Event{Kind: domain.EventUsage, Usage: s.usage})
		}
	case "message_stop":
		s.flushPendingTools()
		if s.finish == "" {
			s.finish = domain.FinishStop
		}
		s.queue = append(s.queue, domain.Event{Kind: domain.EventFinish, FinishReason: s.finish, ProviderMetadata: s.metadata})
		s.done = true
	case "error":
		if event.Error != nil {
			return domain.NewProviderError("anthropic", 0, event.Error.Type, event.Error.Message)
		}
		return domain.NewProviderError("anthropic", 0, "stream_error", "anthropic stream error")
	case "ping":
		return nil
	default:
		return nil
	}
	return nil
}

func (s *stream) flushPendingTools() {
	indices := make([]int, 0, len(s.tools))
	for index := range s.tools {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		acc := s.tools[index]
		args := strings.TrimSpace(acc.args.String())
		if args == "" {
			args = "{}"
		}
		if json.Valid([]byte(args)) {
			s.queue = append(s.queue, domain.Event{Kind: domain.EventToolCall, ToolCall: domain.ToolCall{ID: acc.id, Name: acc.name, Arguments: json.RawMessage(args)}})
		}
	}
	s.tools = make(map[int]*toolAccumulator)
}

func mapStopReason(reason string) domain.FinishReason {
	switch reason {
	case "end_turn", "stop_sequence", "pause_turn":
		return domain.FinishStop
	case "max_tokens", "model_context_window_exceeded":
		return domain.FinishLength
	case "tool_use":
		return domain.FinishToolCalls
	case "refusal":
		return domain.FinishError
	default:
		return domain.FinishOther
	}
}

func (s *stream) Close() error {
	s.closeOnce.Do(func() {
		if s.closer != nil {
			s.closeErr = s.closer.Close()
		}
	})
	return s.closeErr
}
