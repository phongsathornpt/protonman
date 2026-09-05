package model

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

func (c *OfficialOpenAIClient) streamOfficialChat(ctx context.Context, request Request) (Stream, error) {
	params, err := newOfficialChatParams(c.modelID, request)
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "official model stream request started",
		"api", "chat_completions",
		"model", c.modelID,
		"message_count", len(request.Messages),
		"tool_count", len(request.Tools),
	)
	stream := c.sdk.Chat.Completions.NewStreaming(ctx, params)
	return newOfficialChatStream(stream), nil
}

func newOfficialChatParams(modelID string, request Request) (openai.ChatCompletionNewParams, error) {
	messages := make([]openai.ChatCompletionMessageParamUnion, 0, len(request.Messages))
	for _, message := range request.Messages {
		mapped, err := chatMessageParams(message)
		if err != nil {
			return openai.ChatCompletionNewParams{}, err
		}
		messages = append(messages, mapped...)
	}

	var tools []openai.ChatCompletionToolUnionParam
	if len(request.Tools) > 0 {
		tools = make([]openai.ChatCompletionToolUnionParam, 0, len(request.Tools))
		for _, definition := range request.Tools {
			function := shared.FunctionDefinitionParam{
				Name:        definition.Name,
				Description: openai.String(definition.Description),
				Parameters:  shared.FunctionParameters(definition.InputSchema),
			}
			tools = append(tools, openai.ChatCompletionFunctionTool(function))
		}
	}

	choice := openai.ChatCompletionToolChoiceOptionAutoAuto
	if len(request.Tools) == 0 {
		choice = openai.ChatCompletionToolChoiceOptionAutoNone
	}

	return openai.ChatCompletionNewParams{
		Messages: messages,
		Model:    openai.ChatModel(modelID),
		ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.Opt(string(choice)),
		},
		Tools: tools,
	}, nil
}

func chatMessageParams(message Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	switch message.Role {
	case RoleSystem:
		if len(message.Parts) > 0 {
			parts, err := chatTextParts(message.Parts)
			if err != nil {
				return nil, err
			}
			return []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(parts)}, nil
		}
		return []openai.ChatCompletionMessageParamUnion{openai.SystemMessage(message.TextContent())}, nil
	case RoleUser:
		if len(message.Parts) > 0 {
			return []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(chatContentParts(message.Parts)),
			}, nil
		}
		return []openai.ChatCompletionMessageParamUnion{openai.UserMessage(message.TextContent())}, nil
	case RoleAssistant:
		return chatAssistantMessageParams(message)
	case RoleTool:
		return []openai.ChatCompletionMessageParamUnion{
			openai.ToolMessage(message.Content, message.ToolCallID),
		}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported Chat Completions message role %q", ErrInvalidRequest, message.Role)
	}
}

func chatAssistantMessageParams(message Message) ([]openai.ChatCompletionMessageParamUnion, error) {
	if len(message.ToolCalls) == 0 {
		if len(message.Parts) > 0 {
			parts, err := chatTextParts(message.Parts)
			if err != nil {
				return nil, err
			}
			assistant := openai.ChatCompletionAssistantMessageParam{
				Content: openai.ChatCompletionAssistantMessageParamContentUnion{
					OfArrayOfContentParts: make([]openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion, 0, len(parts)),
				},
			}
			for _, part := range parts {
				partCopy := part
				assistant.Content.OfArrayOfContentParts = append(
					assistant.Content.OfArrayOfContentParts,
					openai.ChatCompletionAssistantMessageParamContentArrayOfContentPartUnion{
						OfText: &partCopy,
					},
				)
			}
			return []openai.ChatCompletionMessageParamUnion{{OfAssistant: &assistant}}, nil
		}
		return []openai.ChatCompletionMessageParamUnion{openai.AssistantMessage(message.TextContent())}, nil
	}

	items := make([]openai.ChatCompletionMessageParamUnion, 0, 2)
	if text := message.TextContent(); text != "" {
		items = append(items, openai.AssistantMessage(text))
	}

	assistant := openai.ChatCompletionAssistantMessageParam{
		ToolCalls: make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(message.ToolCalls)),
	}
	for _, call := range message.ToolCalls {
		assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
			OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
				ID: call.ID,
				Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
					Arguments: string(call.Arguments),
					Name:      call.Name,
				},
			},
		})
	}
	items = append(items, openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})
	return items, nil
}

func chatTextParts(parts []ContentPart) ([]openai.ChatCompletionContentPartTextParam, error) {
	textParts := make([]openai.ChatCompletionContentPartTextParam, 0, len(parts))
	for _, part := range parts {
		if part.Type != ContentPartText {
			return nil, fmt.Errorf("%w: Chat Completions only supports image parts on user messages", ErrInvalidRequest)
		}
		if part.Text != "" {
			textParts = append(textParts, openai.ChatCompletionContentPartTextParam{Text: part.Text})
		}
	}
	return textParts, nil
}

func chatContentParts(parts []ContentPart) []openai.ChatCompletionContentPartUnionParam {
	content := make([]openai.ChatCompletionContentPartUnionParam, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case ContentPartText:
			if part.Text != "" {
				content = append(content, openai.TextContentPart(part.Text))
			}
		case ContentPartImage:
			mimeType := part.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			content = append(content, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
				URL: fmt.Sprintf("data:%s;base64,%s", mimeType, part.Data),
			}))
		}
	}
	return content
}

type officialChatSource interface {
	Next() bool
	Current() openai.ChatCompletionChunk
	Err() error
	Close() error
}

type officialChatToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

type officialChatStream struct {
	source      officialChatSource
	toolCalls   map[int64]*officialChatToolCall
	queue       []Event
	done        bool
	terminalErr error
}

func newOfficialChatStream(source officialChatSource) *officialChatStream {
	return &officialChatStream{
		source:    source,
		toolCalls: make(map[int64]*officialChatToolCall),
		queue:     make([]Event, 0),
	}
}

func (s *officialChatStream) Next(ctx context.Context) (Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Event{}, err
		}
		if len(s.queue) > 0 {
			event := s.queue[0]
			s.queue = s.queue[1:]
			return event, nil
		}
		if s.terminalErr != nil {
			return Event{}, s.terminalErr
		}
		if s.done {
			return Event{}, io.EOF
		}
		if !s.source.Next() {
			if err := s.source.Err(); err != nil {
				s.done = true
				return Event{}, fmt.Errorf("official Chat Completions stream: %w", err)
			}
			if err := s.flushToolCalls(); err != nil {
				s.done = true
				s.terminalErr = err
				return Event{}, err
			}
			s.done = true
			s.queue = append(s.queue, Event{Kind: EventDone})
			continue
		}

		if err := s.processChunk(s.source.Current()); err != nil {
			s.done = true
			s.terminalErr = err
			_ = s.source.Close()
		}
	}
}

func (s *officialChatStream) processChunk(chunk openai.ChatCompletionChunk) error {
	for _, choice := range chunk.Choices {
		if choice.Index != 0 {
			return fmt.Errorf("%w: unsupported Chat Completions choice index %d", ErrInvalidEvent, choice.Index)
		}
		if choice.Delta.Content != "" {
			s.queue = append(s.queue, Event{Kind: EventTextDelta, Text: choice.Delta.Content})
		}
		for _, delta := range choice.Delta.ToolCalls {
			call := s.toolCalls[delta.Index]
			if call == nil {
				call = &officialChatToolCall{}
				s.toolCalls[delta.Index] = call
			}
			if delta.ID != "" {
				call.id = delta.ID
			}
			if delta.Function.Name != "" {
				call.name = delta.Function.Name
			}
			if delta.Function.Arguments != "" {
				call.arguments.WriteString(delta.Function.Arguments)
			}
		}
		if choice.FinishReason != "" {
			if err := s.flushToolCalls(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *officialChatStream) flushToolCalls() error {
	indices := make([]int64, 0, len(s.toolCalls))
	for index := range s.toolCalls {
		indices = append(indices, index)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })

	for _, index := range indices {
		call := s.toolCalls[index]
		if call.id == "" || call.name == "" {
			return fmt.Errorf("%w: Chat Completions tool call %d is missing ID or name", ErrInvalidEvent, index)
		}
		arguments := strings.TrimSpace(call.arguments.String())
		if arguments == "" {
			arguments = "{}"
		}
		s.queue = append(s.queue, Event{
			Kind: EventToolCall,
			ToolCall: ToolCall{
				ID:        call.id,
				Name:      call.name,
				Arguments: []byte(arguments),
			},
		})
	}
	s.toolCalls = make(map[int64]*officialChatToolCall)
	return nil
}

func (s *officialChatStream) Close() error {
	if s.source == nil {
		return nil
	}
	return s.source.Close()
}
