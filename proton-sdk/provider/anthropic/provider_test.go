package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func TestAnthropicStreamTextAndRequestMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "secret" {
			t.Fatalf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != DefaultAPIVersion {
			t.Fatalf("anthropic-version = %q", got)
		}
		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "claude-test" || body.MaxTokens != 321 || body.System != "system instruction" {
			t.Fatalf("unexpected body: %#v", body)
		}
		if len(body.Messages) != 1 || body.Messages[0].Role != "user" || len(body.Messages[0].Content) != 2 {
			t.Fatalf("unexpected messages: %#v", body.Messages)
		}
		if body.Messages[0].Content[1].Type != "image" || body.Messages[0].Content[1].Source == nil || body.Messages[0].Content[1].Source.MediaType != "image/png" {
			t.Fatalf("unexpected image block: %#v", body.Messages[0].Content[1])
		}
		if len(body.Tools) != 1 || body.Tools[0].Name != "read_file" {
			t.Fatalf("unexpected tools: %#v", body.Tools)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":10,\"output_tokens\":0,\"cache_read_input_tokens\":2}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	model := NewProvider(ProviderOptions{BaseURL: server.URL, APIKey: "secret"}).Model("claude-test")
	stream, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleSystem, Content: "system instruction"}, {Role: sdk.RoleUser, Parts: []sdk.ContentPart{{Type: sdk.ContentPartText, Text: "look"}, {Type: sdk.ContentPartImage, MIMEType: "image/png", Data: "abc"}}}},
		Tools:    []sdk.Tool{{Name: "read_file", Description: "read file", InputSchema: map[string]any{"type": "object"}}},
		Options:  sdk.ModelOptions{MaxOutputTokens: 321},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "hello" || result.FinishReason != sdk.FinishStop || result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 3 || result.Usage.CachedInputTokens != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAnthropicStreamToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":5,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"toolu_1\",\"name\":\"read_file\",\"input\":{}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"path\\\":\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"\\\"README.md\\\"}\"}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":8}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	model := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("claude-test")
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "inspect"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.FinishReason != sdk.FinishToolCalls || len(result.ToolCalls) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	call := result.ToolCalls[0]
	if call.ID != "toolu_1" || call.Name != "read_file" || string(call.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("unexpected call: %#v", call)
	}
}

func TestAnthropicMapsToolResultToUserBlock(t *testing.T) {
	body, err := buildRequest("claude-test", sdk.Request{Messages: []sdk.Message{
		{Role: sdk.RoleAssistant, ToolCalls: []sdk.ToolCall{{ID: "toolu_1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}}},
		{Role: sdk.RoleTool, ToolCallID: "toolu_1", ToolName: "read_file", Content: "contents"},
	}}, DefaultMaxTokens)
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 2 || body.Messages[0].Content[0].Type != "tool_use" || body.Messages[1].Role != "user" || body.Messages[1].Content[0].Type != "tool_result" || body.Messages[1].Content[0].ToolUseID != "toolu_1" {
		t.Fatalf("unexpected messages: %#v", body.Messages)
	}
}

func TestAnthropicHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()
	model := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("claude-test")
	_, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err == nil {
		t.Fatal("expected error")
	}
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T, want *sdk.ProviderError", err)
	}
	if providerErr.Kind != sdk.ErrorAuthentication || providerErr.StatusCode != http.StatusUnauthorized || providerErr.Code != "authentication_error" {
		t.Fatalf("provider error = %#v", providerErr)
	}
}
func TestAnthropicStreamErrorIsNormalized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n"))
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("claude-test").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, err = stream.Next(context.Background())
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != sdk.ErrorOverloaded || providerErr.Code != "overloaded_error" || !providerErr.Retryable {
		t.Fatalf("stream error = %#v (%v)", providerErr, err)
	}
}
