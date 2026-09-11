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

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type toolAccumulator struct {
	id   string
	name string
	args strings.Builder
}

type stream struct {
	reader     *bufio.Reader
	closer     io.Closer
	queue      []sdk.Event
	tools      map[int]*toolAccumulator
	usage      sdk.Usage
	finish     sdk.FinishReason
	done       bool
	terminal   error
	metadata   sdk.ProviderMetadata
	includeRaw bool
	closeOnce  sync.Once
	closeErr   error
}

func newStream(body io.ReadCloser, metadata sdk.ProviderMetadata, includeRaw bool) *stream {
	return &stream{reader: bufio.NewReader(body), closer: body, tools: make(map[int]*toolAccumulator), metadata: metadata, includeRaw: includeRaw}
}

func (s *stream) Next(ctx context.Context) (sdk.Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return sdk.Event{}, err
		}
		if len(s.queue) > 0 {
			event := s.queue[0]
			s.queue = s.queue[1:]
			return event, nil
		}
		if s.terminal != nil {
			return sdk.Event{}, s.terminal
		}
		if s.done {
			return sdk.Event{}, io.EOF
		}
		line, err := s.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if strings.TrimSpace(line) != "" {
					if parseErr := s.processLine(line); parseErr != nil {
						return sdk.Event{}, parseErr
					}
				}
				if !s.done {
					s.done = true
					s.terminal = fmt.Errorf("%w: anthropic stream closed before message_stop", sdk.ErrIncompleteStream)
				}
				continue
			}
			return sdk.Event{}, fmt.Errorf("read anthropic stream: %w", err)
		}
		if err := s.processLine(line); err != nil {
			if len(s.queue) > 0 {
				s.terminal = err
				event := s.queue[0]
				s.queue = s.queue[1:]
				return event, nil
			}
			return sdk.Event{}, err
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
		s.queue = append(s.queue, sdk.Event{Kind: sdk.EventRaw, RawData: append([]byte(nil), payload...)})
	}
	var event wireEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return fmt.Errorf("%w: decode anthropic event: %w", sdk.ErrInvalidEvent, err)
	}
	switch event.Type {
	case "message_start":
		if event.Message != nil {
			s.usage.InputTokens = event.Message.Usage.InputTokens
			s.usage.OutputTokens = event.Message.Usage.OutputTokens
			s.usage.CachedInputTokens = event.Message.Usage.CacheReadInputTokens
			s.usage.TotalTokens = s.usage.InputTokens + s.usage.OutputTokens
			s.queue = append(s.queue, sdk.Event{Kind: sdk.EventUsage, Usage: s.usage})
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
			return fmt.Errorf("%w: decode anthropic content block: %w", sdk.ErrInvalidEvent, err)
		}
		switch block.Type {
		case "text":
			s.queue = append(s.queue, sdk.Event{Kind: sdk.EventTextStart})
			if block.Text != "" {
				s.queue = append(s.queue, sdk.Event{Kind: sdk.EventTextDelta, Text: block.Text})
			}
		case "tool_use":
			acc := &toolAccumulator{id: block.ID, name: block.Name}
			if len(block.Input) > 0 && string(block.Input) != "{}" {
				acc.args.Write(block.Input)
			}
			s.tools[event.Index] = acc
			s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCallStart, ToolCallID: block.ID, ToolName: block.Name})
		}
	case "content_block_delta":
		var delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
		}
		if err := json.Unmarshal(event.Delta, &delta); err != nil {
			return fmt.Errorf("%w: decode anthropic content delta: %w", sdk.ErrInvalidEvent, err)
		}
		switch delta.Type {
		case "text_delta":
			if delta.Text != "" {
				s.queue = append(s.queue, sdk.Event{Kind: sdk.EventTextDelta, Text: delta.Text})
			}
		case "input_json_delta":
			if acc := s.tools[event.Index]; acc != nil && delta.PartialJSON != "" {
				acc.args.WriteString(delta.PartialJSON)
				s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCallDelta, ToolCallID: acc.id, ToolName: acc.name, ArgumentsDelta: delta.PartialJSON})
			}
		}
	case "content_block_stop":
		if acc := s.tools[event.Index]; acc != nil {
			args := strings.TrimSpace(acc.args.String())
			if args == "" {
				args = "{}"
			}
			if !json.Valid([]byte(args)) {
				return fmt.Errorf("%w: anthropic tool arguments are invalid JSON", sdk.ErrInvalidEvent)
			}
			call := sdk.ToolCall{ID: acc.id, Name: acc.name, Arguments: json.RawMessage(args)}
			s.queue = append(s.queue,
				sdk.Event{Kind: sdk.EventToolCallEnd, ToolCallID: acc.id, ToolName: acc.name},
				sdk.Event{Kind: sdk.EventToolCall, ToolCall: call},
			)
			delete(s.tools, event.Index)
		} else {
			s.queue = append(s.queue, sdk.Event{Kind: sdk.EventTextEnd})
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
			s.queue = append(s.queue, sdk.Event{Kind: sdk.EventUsage, Usage: s.usage})
		}
	case "message_stop":
		s.flushPendingTools()
		if s.finish == "" {
			s.finish = sdk.FinishStop
		}
		s.queue = append(s.queue, sdk.Event{Kind: sdk.EventFinish, FinishReason: s.finish, ProviderMetadata: s.metadata})
		s.done = true
	case "error":
		if event.Error != nil {
			return sdk.NewProviderError("anthropic", 0, event.Error.Type, event.Error.Message)
		}
		return sdk.NewProviderError("anthropic", 0, "stream_error", "anthropic stream error")
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
			s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCall, ToolCall: sdk.ToolCall{ID: acc.id, Name: acc.name, Arguments: json.RawMessage(args)}})
		}
	}
	s.tools = make(map[int]*toolAccumulator)
}

func mapStopReason(reason string) sdk.FinishReason {
	switch reason {
	case "end_turn", "stop_sequence", "pause_turn":
		return sdk.FinishStop
	case "max_tokens", "model_context_window_exceeded":
		return sdk.FinishLength
	case "tool_use":
		return sdk.FinishToolCalls
	case "refusal":
		return sdk.FinishError
	default:
		return sdk.FinishOther
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
