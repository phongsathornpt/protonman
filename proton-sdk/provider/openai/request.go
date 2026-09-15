package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"github.com/phongsathornpt/protonman/proton-sdk/internal/providerutil"
)

type chatToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function chatFunctionCall `json:"function"`
}
type chatFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type chatMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
}
type chatTool struct {
	Type     string       `json:"type"`
	Function chatFunction `json:"function"`
}
type chatFunction struct {
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Parameters      map[string]any  `json:"parameters,omitempty"`
	ProviderOptions json.RawMessage `json:"-"`
}

func (f chatFunction) MarshalJSON() ([]byte, error) {
	base := struct {
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters,omitempty"`
	}{f.Name, f.Description, f.Parameters}
	return providerutil.MarshalWithOptions(base, f.ProviderOptions, "name", "description", "parameters")
}

type chatRequest struct {
	Model           string              `json:"model"`
	Messages        []chatMessage       `json:"messages"`
	Stream          bool                `json:"stream"`
	Tools           []chatTool          `json:"tools,omitempty"`
	ToolChoice      string              `json:"tool_choice,omitempty"`
	MaxTokens       int                 `json:"max_tokens,omitempty"`
	ReasoningEffort sdk.ReasoningEffort `json:"reasoning_effort,omitempty"`
	EnableThinking  *bool               `json:"enable_thinking,omitempty"`
}

type responsesTool struct {
	Type            string          `json:"type"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Parameters      map[string]any  `json:"parameters,omitempty"`
	ProviderOptions json.RawMessage `json:"-"`
}

func (t responsesTool) MarshalJSON() ([]byte, error) {
	base := struct {
		Type        string         `json:"type"`
		Name        string         `json:"name"`
		Description string         `json:"description,omitempty"`
		Parameters  map[string]any `json:"parameters,omitempty"`
	}{t.Type, t.Name, t.Description, t.Parameters}
	return providerutil.MarshalWithOptions(base, t.ProviderOptions, "type", "name", "description", "parameters")
}

type reasoningConfig struct {
	Effort sdk.ReasoningEffort `json:"effort"`
}

type responsesRequest struct {
	Model           string           `json:"model"`
	Stream          bool             `json:"stream"`
	Input           []any            `json:"input"`
	Tools           []responsesTool  `json:"tools,omitempty"`
	ToolChoice      string           `json:"tool_choice,omitempty"`
	MaxOutputTokens int              `json:"max_output_tokens,omitempty"`
	Reasoning       *reasoningConfig `json:"reasoning,omitempty"`
}

func (m *LanguageModel) encodeRequest(request sdk.Request) (string, []byte, error) {
	if m.useResponsesAPI || strings.HasSuffix(m.provider.options.BaseURL, "/responses") {
		endpoint := m.provider.options.BaseURL
		if strings.HasSuffix(endpoint, "/chat/completions") {
			endpoint = strings.TrimSuffix(endpoint, "/chat/completions") + "/responses"
		} else if !strings.HasSuffix(endpoint, "/responses") {
			endpoint += "/responses"
		}
		input := make([]any, 0, len(request.Messages))
		for _, message := range request.Messages {
			switch message.Role {
			case sdk.RoleUser, sdk.RoleSystem:
				input = append(input, map[string]any{"role": string(message.Role), "content": responsesMessageContent(message)})
			case sdk.RoleAssistant:
				if text := strings.TrimSpace(message.TextContent()); text != "" {
					input = append(input, map[string]any{"role": "assistant", "content": text})
				}
				for _, call := range message.ToolCalls {
					input = append(input, map[string]any{"type": "function_call", "name": call.Name, "call_id": call.ID, "arguments": string(call.Arguments)})
				}
			case sdk.RoleTool:
				input = append(input, map[string]any{"type": "function_call_output", "call_id": message.ToolCallID, "output": message.TextContent()})
			}
		}
		tools := make([]responsesTool, 0, len(request.Tools))
		for _, tool := range request.Tools {
			tools = append(tools, responsesTool{Type: "function", Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema, ProviderOptions: tool.ProviderOptions["openai"]})
		}
		encoded, err := providerutil.MarshalWithOptions(responsesRequest{Model: m.modelID, Stream: true, Input: input, Tools: tools, ToolChoice: toolChoice(len(tools), request.Options.ToolChoice), MaxOutputTokens: request.Options.MaxOutputTokens, Reasoning: responseReasoning(request.Options.ReasoningEffort)}, request.Options.ProviderOptions["openai"], "model", "stream", "input", "tools", "tool_choice", "max_output_tokens", "reasoning")
		if err != nil {
			return "", nil, fmt.Errorf("marshal responses request: %w", err)
		}
		return endpoint, encoded, nil
	}

	endpoint := m.provider.options.BaseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	messages := make([]chatMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role == sdk.RoleAssistant && len(message.ToolCalls) > 0 {
			calls := make([]chatToolCall, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				calls = append(calls, chatToolCall{ID: call.ID, Type: "function", Function: chatFunctionCall{Name: call.Name, Arguments: string(call.Arguments)}})
			}
			msg := chatMessage{Role: "assistant", ToolCalls: calls}
			if text := strings.TrimSpace(message.TextContent()); text != "" {
				content := text
				msg.Content = &content
			}
			messages = append(messages, msg)
			continue
		}
		msg := chatMessage{Role: string(message.Role)}
		if len(message.Parts) > 0 {
			msg.Content = chatContentParts(message.Parts)
		} else {
			content := message.Content
			msg.Content = &content
		}
		if message.Role == sdk.RoleTool {
			msg.ToolCallID = message.ToolCallID
		}
		messages = append(messages, msg)
	}
	tools := make([]chatTool, 0, len(request.Tools))
	for _, tool := range request.Tools {
		tools = append(tools, chatTool{Type: "function", Function: chatFunction{Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema, ProviderOptions: tool.ProviderOptions["openai"]}})
	}
	reasoningEffort, enableThinking := m.qwenChatReasoning(request.Options.ReasoningEffort)
	encoded, err := providerutil.MarshalWithOptions(chatRequest{Model: m.modelID, Messages: messages, Stream: true, Tools: tools, ToolChoice: toolChoice(len(tools), request.Options.ToolChoice), MaxTokens: request.Options.MaxOutputTokens, ReasoningEffort: reasoningEffort, EnableThinking: enableThinking}, request.Options.ProviderOptions["openai"], "model", "messages", "stream", "tools", "tool_choice", "max_tokens", "reasoning_effort", "enable_thinking")
	if err != nil {
		return "", nil, fmt.Errorf("marshal chat request: %w", err)
	}
	return endpoint, encoded, nil
}

func responsesMessageContent(message sdk.Message) any {
	if len(message.Parts) == 0 {
		return message.TextContent()
	}
	content := make([]map[string]any, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch part.Type {
		case sdk.ContentPartText:
			if part.Text != "" {
				content = append(content, map[string]any{"type": "input_text", "text": part.Text})
			}
		case sdk.ContentPartImage:
			if part.Data == "" {
				continue
			}
			mime := strings.TrimSpace(part.MIMEType)
			if mime == "" {
				mime = "image/png"
			}
			content = append(content, map[string]any{
				"type":      "input_image",
				"image_url": fmt.Sprintf("data:%s;base64,%s", mime, part.Data),
			})
		}
	}
	if len(content) == 0 {
		return message.TextContent()
	}
	return content
}

func (m *LanguageModel) qwenChatReasoning(effort sdk.ReasoningEffort) (sdk.ReasoningEffort, *bool) {
	if effort != sdk.ReasoningNone || !isDashScopeBaseURL(m.provider.options.BaseURL) || !isQwenHybridThinkingModel(m.modelID) {
		return effort, nil
	}
	disabled := false
	return sdk.ReasoningDefault, &disabled
}

func isDashScopeBaseURL(baseURL string) bool {
	host := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(host, "dashscope.aliyuncs.com") || strings.Contains(host, "dashscope-intl.aliyuncs.com")
}

func isQwenHybridThinkingModel(modelID string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if slash := strings.LastIndexByte(id, '/'); slash >= 0 {
		id = id[slash+1:]
	}
	for _, prefix := range []string{"qwen3.5-plus", "qwen3.6-plus", "qwen3.6-flash", "qwen3.7-plus", "qwen3.7-max", "qwen3.8-flash"} {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

func chatContentParts(parts []sdk.ContentPart) []map[string]any {
	content := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case sdk.ContentPartText:
			if part.Text != "" {
				content = append(content, map[string]any{"type": "text", "text": part.Text})
			}
		case sdk.ContentPartImage:
			mime := part.MIMEType
			if mime == "" {
				mime = "image/png"
			}
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": fmt.Sprintf("data:%s;base64,%s", mime, part.Data)}})
		}
	}
	return content
}

func responseReasoning(effort sdk.ReasoningEffort) *reasoningConfig {
	if effort == sdk.ReasoningDefault {
		return nil
	}
	return &reasoningConfig{Effort: effort}
}

func toolChoice(count int, choice sdk.ToolChoice) string {
	if count == 0 {
		return "none"
	}
	if choice == sdk.ToolChoiceRequired {
		return "required"
	}
	return ""
}
