package model

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// OfficialOpenAIClient adapts the official openai-go client to Proton's
// provider-neutral model boundary.
type OfficialOpenAIClient struct {
	openAIClientConfig
	sdk openai.Client
}

var _ Client = (*OfficialOpenAIClient)(nil)

// NewOfficialOpenAIClient creates an official openai-go client without changing
// the currently wired compatible-provider client. Responses models are enabled
// first; Chat Completions remains on the legacy path until its adapter lands.
func NewOfficialOpenAIClient(
	baseURL string,
	apiKey string,
	modelID string,
	opts ...OpenAIOption,
) *OfficialOpenAIClient {
	client := &OfficialOpenAIClient{
		openAIClientConfig: newOpenAIClientConfig(baseURL, apiKey, modelID),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&client.openAIClientConfig)
		}
	}

	client.sdk = openai.NewClient(officialOpenAIOptions(client.openAIClientConfig)...)
	return client
}

// Stream starts an official Responses or Chat Completions stream.
func (c *OfficialOpenAIClient) Stream(ctx context.Context, request Request) (Stream, error) {
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if isResponsesModel(c.modelID) || strings.HasSuffix(c.baseURL, "/responses") {
		params, err := newOfficialResponsesParams(c.modelID, request)
		if err != nil {
			return nil, err
		}

		slog.DebugContext(ctx, "official model stream request started",
			"api", "responses",
			"model", c.modelID,
			"message_count", len(request.Messages),
			"tool_count", len(request.Tools),
		)
		stream := c.sdk.Responses.NewStreaming(ctx, params)
		return newOfficialResponsesStream(stream), nil
	}

	return c.streamOfficialChat(ctx, request)
}

func newOfficialResponsesParams(modelID string, request Request) (responses.ResponseNewParams, error) {
	input := make([]responses.ResponseInputItemUnionParam, 0, len(request.Messages)+1)
	for _, message := range request.Messages {
		items, err := responsesInputItems(message)
		if err != nil {
			return responses.ResponseNewParams{}, err
		}
		input = append(input, items...)
	}

	var tools []responses.ToolUnionParam
	if len(request.Tools) > 0 {
		tools = make([]responses.ToolUnionParam, 0, len(request.Tools))
	}
	for _, definition := range request.Tools {
		tools = append(tools, responses.ToolParamOfFunction(
			definition.Name,
			definition.InputSchema,
			false,
		))
		if tool := tools[len(tools)-1].OfFunction; tool != nil {
			tool.Description = openai.String(definition.Description)
		}
	}

	return responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: responses.ResponseInputParam(input),
		},
		Model:      openai.ResponsesModel(modelID),
		ToolChoice: officialResponsesToolChoice(len(request.Tools)),
		Tools:      tools,
	}, nil
}

func officialResponsesToolChoice(toolCount int) responses.ResponseNewParamsToolChoiceUnion {
	choice := responses.ToolChoiceOptionsAuto
	if toolCount == 0 {
		choice = responses.ToolChoiceOptionsNone
	}
	return responses.ResponseNewParamsToolChoiceUnion{
		OfToolChoiceMode: param.NewOpt(choice),
	}
}

func responsesInputItems(message Message) ([]responses.ResponseInputItemUnionParam, error) {
	if message.Role == RoleAssistant && len(message.ToolCalls) > 0 {
		items := make([]responses.ResponseInputItemUnionParam, 0, len(message.ToolCalls)+1)
		if text := message.TextContent(); text != "" {
			items = append(items, responses.ResponseInputItemParamOfMessage(
				text,
				responses.EasyInputMessageRoleAssistant,
			))
		}
		for _, call := range message.ToolCalls {
			items = append(items, responses.ResponseInputItemParamOfFunctionCall(
				string(call.Arguments),
				call.ID,
				call.Name,
			))
		}
		return items, nil
	}

	switch message.Role {
	case RoleSystem:
		return []responses.ResponseInputItemUnionParam{responseMessageItem(
			message,
			responses.EasyInputMessageRoleSystem,
		)}, nil
	case RoleUser:
		return []responses.ResponseInputItemUnionParam{responseMessageItem(
			message,
			responses.EasyInputMessageRoleUser,
		)}, nil
	case RoleAssistant:
		return []responses.ResponseInputItemUnionParam{responseMessageItem(
			message,
			responses.EasyInputMessageRoleAssistant,
		)}, nil
	case RoleTool:
		item := responses.ResponseInputItemParamOfFunctionCallOutput(message.Content)
		if item.OfFunctionCallOutput == nil {
			return nil, fmt.Errorf("%w: create Responses tool output", ErrInvalidRequest)
		}
		item.OfFunctionCallOutput.CallID = openai.String(message.ToolCallID)
		return []responses.ResponseInputItemUnionParam{item}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported Responses message role %q", ErrInvalidRequest, message.Role)
	}
}

func responseMessageItem(
	message Message,
	role responses.EasyInputMessageRole,
) responses.ResponseInputItemUnionParam {
	if len(message.Parts) == 0 {
		return responses.ResponseInputItemParamOfMessage(message.TextContent(), role)
	}
	return responses.ResponseInputItemParamOfMessage(responsesMessageContent(message), role)
}

func responsesMessageContent(message Message) responses.ResponseInputMessageContentListParam {
	parts := make(responses.ResponseInputMessageContentListParam, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch part.Type {
		case ContentPartText:
			if part.Text != "" {
				parts = append(parts, responses.ResponseInputContentParamOfInputText(part.Text))
			}
		case ContentPartImage:
			mimeType := part.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			image := responses.ResponseInputImageParam{
				Detail:   responses.ResponseInputImageDetailAuto,
				ImageURL: openai.String(fmt.Sprintf("data:%s;base64,%s", mimeType, part.Data)),
			}
			parts = append(parts, responses.ResponseInputContentUnionParam{
				OfInputImage: &image,
			})
		}
	}
	return parts
}

type officialResponsesSource interface {
	Next() bool
	Current() responses.ResponseStreamEventUnion
	Err() error
	Close() error
}

type officialResponsesToolCall struct {
	id        string
	name      string
	arguments strings.Builder
}

type officialResponsesStream struct {
	source      officialResponsesSource
	toolCalls   map[string]*officialResponsesToolCall
	queue       []Event
	done        bool
	terminalErr error
}

func newOfficialResponsesStream(source officialResponsesSource) *officialResponsesStream {
	return &officialResponsesStream{
		source:    source,
		toolCalls: make(map[string]*officialResponsesToolCall),
		queue:     make([]Event, 0),
	}
}

func (s *officialResponsesStream) Next(ctx context.Context) (Event, error) {
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
			s.done = true
			if err := s.source.Err(); err != nil {
				return Event{}, fmt.Errorf("official Responses stream: %w", err)
			}
			return Event{}, fmt.Errorf("%w: official Responses stream ended before response.completed", ErrIncompleteStream)
		}

		if err := s.processEvent(s.source.Current()); err != nil {
			s.done = true
			s.terminalErr = err
			_ = s.source.Close()
		}
	}
}

func (s *officialResponsesStream) processEvent(event responses.ResponseStreamEventUnion) error {
	switch event.Type {
	case "response.output_text.delta":
		if event.Delta != "" {
			s.queue = append(s.queue, Event{Kind: EventTextDelta, Text: event.Delta})
		}
	case "response.output_item.added":
		return s.addToolCall(event.Item)
	case "response.function_call_arguments.delta":
		call, ok := s.toolCalls[event.ItemID]
		if !ok {
			return fmt.Errorf("%w: function call delta references unknown item %q", ErrInvalidEvent, event.ItemID)
		}
		call.arguments.WriteString(event.Delta)
	case "response.function_call_arguments.done":
		call, ok := s.toolCalls[event.ItemID]
		if !ok {
			return fmt.Errorf("%w: function call completion references unknown item %q", ErrInvalidEvent, event.ItemID)
		}
		call.name = event.Name
		call.arguments.Reset()
		call.arguments.WriteString(event.Arguments)
	case "response.output_item.done":
		return s.finishToolCall(event.Item)
	case "response.completed":
		if err := s.flushPendingToolCalls(); err != nil {
			return err
		}
		s.done = true
		s.queue = append(s.queue, Event{Kind: EventDone})
	case "response.failed", "response.incomplete":
		return fmt.Errorf("official Responses stream: %s", event.Type)
	case "error":
		message := event.Message
		if message == "" {
			message = "provider returned an error event"
		}
		return fmt.Errorf("official Responses stream: %s", message)
	}
	return nil
}

func (s *officialResponsesStream) addToolCall(item responses.ResponseOutputItemUnion) error {
	if item.Type != "function_call" {
		return nil
	}
	if item.ID == "" {
		return fmt.Errorf("%w: function call output item has no item ID", ErrInvalidEvent)
	}
	call := s.toolCalls[item.ID]
	if call == nil {
		call = &officialResponsesToolCall{}
		s.toolCalls[item.ID] = call
	}
	if item.CallID != "" {
		call.id = item.CallID
	}
	if item.Name != "" {
		call.name = item.Name
	}
	if item.Arguments.OfString != "" {
		call.arguments.WriteString(item.Arguments.OfString)
	}
	return nil
}

func (s *officialResponsesStream) flushPendingToolCalls() error {
	if len(s.toolCalls) == 0 {
		return nil
	}
	itemIDs := make([]string, 0, len(s.toolCalls))
	for itemID := range s.toolCalls {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Strings(itemIDs)
	for _, itemID := range itemIDs {
		call := s.toolCalls[itemID]
		if call == nil || call.id == "" || call.name == "" {
			return fmt.Errorf("%w: pending function call %q is missing ID or name", ErrInvalidEvent, itemID)
		}
		arguments := strings.TrimSpace(call.arguments.String())
		if arguments == "" {
			arguments = "{}"
		}
		s.queue = append(s.queue, Event{
			Kind: EventToolCall,
			ToolCall: ToolCall{
				ID: call.id, Name: call.name, Arguments: []byte(arguments),
			},
		})
		delete(s.toolCalls, itemID)
	}
	return nil
}

func (s *officialResponsesStream) finishToolCall(item responses.ResponseOutputItemUnion) error {
	if item.Type != "function_call" {
		return nil
	}
	if item.ID == "" {
		return fmt.Errorf("%w: completed function call has no item ID", ErrInvalidEvent)
	}
	call := s.toolCalls[item.ID]
	if call == nil {
		call = &officialResponsesToolCall{}
	}
	if item.CallID != "" {
		call.id = item.CallID
	}
	if item.Name != "" {
		call.name = item.Name
	}
	if item.Arguments.OfString != "" {
		call.arguments.Reset()
		call.arguments.WriteString(item.Arguments.OfString)
	}
	if call.id == "" || call.name == "" {
		return fmt.Errorf("%w: completed function call %q is missing ID or name", ErrInvalidEvent, item.ID)
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
	delete(s.toolCalls, item.ID)
	return nil
}

func (s *officialResponsesStream) Close() error {
	if s.source == nil {
		return nil
	}
	return s.source.Close()
}

func officialOpenAIOptions(config openAIClientConfig) []option.RequestOption {
	requestOptions := []option.RequestOption{
		option.WithAPIKey(config.apiKey),
		option.WithBaseURL(officialOpenAIBaseURL(config.baseURL)),
		option.WithMaxRetries(2),
	}
	if config.httpClient != nil {
		requestOptions = append(requestOptions,
			option.WithHTTPClient(config.httpClient),
			option.WithRequestTimeout(config.httpClient.Timeout),
		)
	}
	if config.userAgent != "" {
		requestOptions = append(requestOptions, option.WithHeader("User-Agent", config.userAgent))
	}
	if config.sessionID != "" {
		requestOptions = append(requestOptions,
			option.WithHeader("x-session-affinity", config.sessionID),
			option.WithHeader("X-Session-Id", config.sessionID),
		)
	}
	return requestOptions
}

func officialOpenAIBaseURL(baseURL string) string {
	for _, suffix := range []string{"/chat/completions", "/responses"} {
		if strings.HasSuffix(baseURL, suffix) {
			return strings.TrimSuffix(baseURL, suffix)
		}
	}
	return baseURL
}

// SessionID returns the configured session identifier.
func (c *OfficialOpenAIClient) SessionID() string {
	return c.sessionID
}

// SetSessionID updates the session identifier for future requests.
func (c *OfficialOpenAIClient) SetSessionID(sessionID string) {
	c.sessionID = sessionID
	requestOptions := officialOpenAIOptions(c.openAIClientConfig)
	c.sdk = openai.NewClient(requestOptions...)
}
