package openai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type accumulatedToolCall struct {
	id        string
	name      string
	arguments strings.Builder
	started   bool
}

type stream struct {
	reader        *bufio.Reader
	closer        io.Closer
	chatCalls     map[int]*accumulatedToolCall
	responseCalls map[string]*accumulatedToolCall
	queue         []sdk.Event
	done          bool
	terminalErr   error
	generatedSeq  uint64
	metadata      sdk.ProviderMetadata
	includeRaw    bool
	provider      string
}

func newStream(body io.ReadCloser, metadata sdk.ProviderMetadata, includeRaw bool, provider string) *stream {
	return &stream{reader: bufio.NewReader(body), closer: body, chatCalls: map[int]*accumulatedToolCall{}, responseCalls: map[string]*accumulatedToolCall{}, metadata: metadata, includeRaw: includeRaw, provider: provider}
}

type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content,omitempty"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function,omitempty"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage,omitempty"`
	Error *struct {
		Message  string         `json:"message"`
		Type     string         `json:"type,omitempty"`
		Code     string         `json:"code,omitempty"`
		Metadata map[string]any `json:"metadata,omitempty"`
	} `json:"error,omitempty"`
}

type responsesChunk struct {
	Type      string `json:"type"`
	Delta     string `json:"delta,omitempty"`
	ItemID    string `json:"item_id,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Item      *struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		Name      string `json:"name,omitempty"`
		CallID    string `json:"call_id,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"item,omitempty"`
	Response *struct {
		Error *struct {
			Message  string         `json:"message"`
			Type     string         `json:"type,omitempty"`
			Code     string         `json:"code,omitempty"`
			Metadata map[string]any `json:"metadata,omitempty"`
		} `json:"error,omitempty"`
		Usage *struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage,omitempty"`
	} `json:"response,omitempty"`
	Error *struct {
		Message  string         `json:"message"`
		Type     string         `json:"type,omitempty"`
		Code     string         `json:"code,omitempty"`
		Metadata map[string]any `json:"metadata,omitempty"`
	} `json:"error,omitempty"`
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
		if s.terminalErr != nil {
			return sdk.Event{}, s.terminalErr
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
					s.terminalErr = fmt.Errorf("%w: provider closed before terminal event", sdk.ErrIncompleteStream)
				}
				if len(s.queue) > 0 {
					event := s.queue[0]
					s.queue = s.queue[1:]
					return event, nil
				}
				if s.terminalErr != nil {
					return sdk.Event{}, s.terminalErr
				}
				return sdk.Event{}, io.EOF
			}
			return sdk.Event{}, fmt.Errorf("read stream: %w", err)
		}
		if err := s.processLine(line); err != nil {
			if len(s.queue) > 0 {
				s.terminalErr = err
				event := s.queue[0]
				s.queue = s.queue[1:]
				return event, nil
			}
			return sdk.Event{}, err
		}
	}
}

func (s *stream) processLine(line string) error {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") {
		return nil
	}
	if !strings.HasPrefix(line, "data:") {
		return nil
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	if s.includeRaw && payload != "" {
		s.queue = append(s.queue, sdk.Event{Kind: sdk.EventRaw, RawData: append([]byte(nil), payload...)})
	}
	if payload == "[DONE]" {
		s.finish(sdk.FinishStop)
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &fields); err != nil {
		return fmt.Errorf("%w: decode SSE data: %w", sdk.ErrInvalidEvent, err)
	}
	if _, ok := fields["choices"]; ok {
		return s.processChat(payload)
	}
	if _, ok := fields["error"]; ok {
		var chunk chatChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("%w: decode provider error: %w", sdk.ErrInvalidEvent, err)
		}
		if chunk.Error != nil {
			return providerStreamError(s.provider, chunk.Error.Code, chunk.Error.Type, chunk.Error.Message, chunk.Error.Metadata)
		}
	}
	if _, ok := fields["type"]; ok {
		return s.processResponses(payload)
	}
	return fmt.Errorf("%w: unsupported SSE data payload", sdk.ErrInvalidEvent)
}

func (s *stream) processChat(payload string) error {
	var chunk chatChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return fmt.Errorf("%w: decode chat completion event: %w", sdk.ErrInvalidEvent, err)
	}
	if chunk.Error != nil {
		return providerStreamError(s.provider, chunk.Error.Code, chunk.Error.Type, chunk.Error.Message, chunk.Error.Metadata)
	}
	if chunk.Usage != nil {
		s.queue = append(s.queue, sdk.Event{Kind: sdk.EventUsage, Usage: sdk.Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens, TotalTokens: chunk.Usage.TotalTokens}})
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.Content != "" {
			s.queue = append(s.queue, sdk.Event{Kind: sdk.EventTextDelta, Text: choice.Delta.Content})
		}
		for _, toolDelta := range choice.Delta.ToolCalls {
			call := s.chatCalls[toolDelta.Index]
			if call == nil {
				call = &accumulatedToolCall{}
				s.chatCalls[toolDelta.Index] = call
			}
			if toolDelta.ID != "" {
				call.id = toolDelta.ID
			}
			if toolDelta.Function.Name != "" {
				call.name = toolDelta.Function.Name
			}
			if !call.started && call.id != "" && call.name != "" {
				call.started = true
				s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCallStart, ToolCallID: call.id, ToolName: call.name})
			}
			if toolDelta.Function.Arguments != "" {
				call.arguments.WriteString(toolDelta.Function.Arguments)
				if call.id != "" {
					s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCallDelta, ToolCallID: call.id, ArgumentsDelta: toolDelta.Function.Arguments})
				}
			}
		}
		if choice.FinishReason != nil {
			s.finish(mapFinishReason(*choice.FinishReason))
		}
	}
	return nil
}

func (s *stream) processResponses(payload string) error {
	var chunk responsesChunk
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return fmt.Errorf("%w: decode responses event: %w", sdk.ErrInvalidEvent, err)
	}
	if chunk.Error != nil {
		return providerStreamError(s.provider, chunk.Error.Code, chunk.Error.Type, chunk.Error.Message, chunk.Error.Metadata)
	}
	if chunk.Response != nil && chunk.Response.Error != nil {
		return providerStreamError(s.provider, chunk.Response.Error.Code, chunk.Response.Error.Type, chunk.Response.Error.Message, chunk.Response.Error.Metadata)
	}
	switch chunk.Type {
	case "response.output_text.delta":
		if chunk.Delta != "" {
			s.queue = append(s.queue, sdk.Event{Kind: sdk.EventTextDelta, Text: chunk.Delta})
		}
	case "response.output_item.added":
		if chunk.Item != nil && chunk.Item.Type == "function_call" {
			callID := chunk.Item.CallID
			if callID == "" {
				callID = chunk.Item.ID
			}
			call := &accumulatedToolCall{id: callID, name: chunk.Item.Name, started: callID != "" && chunk.Item.Name != ""}
			s.responseCalls[chunk.Item.ID] = call
			if call.started {
				s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCallStart, ToolCallID: call.id, ToolName: call.name})
			}
		}
	case "response.function_call_arguments.delta":
		if call := s.responseCalls[chunk.ItemID]; call != nil && chunk.Delta != "" {
			call.arguments.WriteString(chunk.Delta)
			if call.id != "" {
				s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCallDelta, ToolCallID: call.id, ArgumentsDelta: chunk.Delta})
			}
		}
	case "response.output_item.done":
		if chunk.Item != nil && chunk.Item.Type == "function_call" {
			s.finishResponseCall(chunk)
		}
	case "response.completed":
		if chunk.Response != nil && chunk.Response.Usage != nil {
			u := chunk.Response.Usage
			s.queue = append(s.queue, sdk.Event{Kind: sdk.EventUsage, Usage: sdk.Usage{InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, TotalTokens: u.TotalTokens}})
		}
		s.finish(sdk.FinishStop)
	}
	return nil
}

func (s *stream) finishResponseCall(chunk responsesChunk) {
	callID := chunk.Item.CallID
	if callID == "" {
		callID = chunk.Item.ID
	}
	name := chunk.Item.Name
	arguments := strings.TrimSpace(chunk.Item.Arguments)
	if call := s.responseCalls[chunk.Item.ID]; call != nil {
		if callID == "" {
			callID = call.id
		}
		if name == "" {
			name = call.name
		}
		if arguments == "" {
			arguments = strings.TrimSpace(call.arguments.String())
		}
		delete(s.responseCalls, chunk.Item.ID)
	}
	if callID == "" {
		callID = s.nextCallID()
	}
	if arguments == "" {
		arguments = "{}"
	}
	s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCallEnd, ToolCallID: callID}, sdk.Event{Kind: sdk.EventToolCall, ToolCall: sdk.ToolCall{ID: callID, Name: name, Arguments: json.RawMessage(arguments)}})
}

func (s *stream) finish(reason sdk.FinishReason) {
	if s.done {
		return
	}
	s.flushCalls()
	s.queue = append(s.queue, sdk.Event{Kind: sdk.EventFinish, FinishReason: reason, ProviderMetadata: s.metadata})
	s.done = true
}

func (s *stream) flushCalls() {
	indices := make([]int, 0, len(s.chatCalls))
	for index := range s.chatCalls {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for _, index := range indices {
		s.emitCompleteCall(s.chatCalls[index])
		delete(s.chatCalls, index)
	}
	ids := make([]string, 0, len(s.responseCalls))
	for id := range s.responseCalls {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		call := s.responseCalls[id]
		if call.id == "" {
			call.id = id
		}
		s.emitCompleteCall(call)
		delete(s.responseCalls, id)
	}
}

func (s *stream) emitCompleteCall(call *accumulatedToolCall) {
	if call == nil {
		return
	}
	if call.id == "" {
		call.id = s.nextCallID()
	}
	arguments := strings.TrimSpace(call.arguments.String())
	if arguments == "" {
		arguments = "{}"
	}
	if call.started {
		s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCallEnd, ToolCallID: call.id})
	}
	s.queue = append(s.queue, sdk.Event{Kind: sdk.EventToolCall, ToolCall: sdk.ToolCall{ID: call.id, Name: call.name, Arguments: json.RawMessage(arguments)}})
}

func (s *stream) nextCallID() string {
	s.generatedSeq++
	return fmt.Sprintf("generated_call_%d", s.generatedSeq)
}
func (s *stream) Close() error {
	if s.closer != nil {
		return s.closer.Close()
	}
	return nil
}

func mapFinishReason(reason string) sdk.FinishReason {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "stop":
		return sdk.FinishStop
	case "length":
		return sdk.FinishLength
	case "tool_calls", "function_call":
		return sdk.FinishToolCalls
	case "content_filter":
		return sdk.FinishOther
	default:
		return sdk.FinishOther
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
