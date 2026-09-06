package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

type requestBody struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
	Tools     []toolDef `json:"tools,omitempty"`
	Stream    bool      `json:"stream"`
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Source    *imageSource    `json:"source,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

func buildRequest(modelID string, request sdk.Request, defaultMaxTokens int) (requestBody, error) {
	if err := request.Validate(); err != nil {
		return requestBody{}, err
	}
	if strings.TrimSpace(modelID) == "" {
		return requestBody{}, fmt.Errorf("%w: model id is required", sdk.ErrInvalidRequest)
	}

	maxTokens := request.Options.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	body := requestBody{Model: modelID, MaxTokens: maxTokens, Stream: true}
	var systems []string
	for _, source := range request.Messages {
		switch source.Role {
		case sdk.RoleSystem:
			if text := strings.TrimSpace(source.TextContent()); text != "" {
				systems = append(systems, text)
			}
		case sdk.RoleUser:
			body.Messages = append(body.Messages, message{Role: "user", Content: userContent(source)})
		case sdk.RoleAssistant:
			body.Messages = append(body.Messages, message{Role: "assistant", Content: assistantContent(source)})
		case sdk.RoleTool:
			body.Messages = append(body.Messages, message{Role: "user", Content: []contentBlock{{
				Type: "tool_result", ToolUseID: source.ToolCallID, Content: source.TextContent(),
			}}})
		}
	}
	body.System = strings.Join(systems, "\n\n")
	for _, tool := range request.Tools {
		schema := tool.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		body.Tools = append(body.Tools, toolDef{Name: tool.Name, Description: tool.Description, InputSchema: schema})
	}
	return body, nil
}

func userContent(source sdk.Message) []contentBlock {
	if len(source.Parts) == 0 {
		return []contentBlock{{Type: "text", Text: source.Content}}
	}
	blocks := make([]contentBlock, 0, len(source.Parts))
	for _, part := range source.Parts {
		switch part.Type {
		case sdk.ContentPartText:
			if part.Text != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: part.Text})
			}
		case sdk.ContentPartImage:
			mediaType := strings.TrimSpace(part.MIMEType)
			if mediaType == "" {
				mediaType = "image/png"
			}
			blocks = append(blocks, contentBlock{Type: "image", Source: &imageSource{Type: "base64", MediaType: mediaType, Data: part.Data}})
		}
	}
	return blocks
}

func assistantContent(source sdk.Message) []contentBlock {
	blocks := make([]contentBlock, 0, 1+len(source.ToolCalls))
	if text := source.TextContent(); text != "" {
		blocks = append(blocks, contentBlock{Type: "text", Text: text})
	}
	for _, call := range source.ToolCalls {
		input := call.Arguments
		if len(input) == 0 {
			input = json.RawMessage(`{}`)
		}
		blocks = append(blocks, contentBlock{Type: "tool_use", ID: call.ID, Name: call.Name, Input: input})
	}
	return blocks
}
