package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/phongsathornpt/proton/proton-sdk"
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
		w.Header().Set("Request-Id", "req-anthropic-1")
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
	if got := string(result.ProviderMetadata["anthropic"]); !strings.Contains(got, `"request_id":"req-anthropic-1"`) {
		t.Fatalf("provider metadata = %s", got)
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
		{Role: sdk.RoleTool, ToolCallID: "toolu_1", ToolName: "read_file", Content: "failed", ToolResultIsError: true},
	}}, DefaultMaxTokens)
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Messages) != 2 || body.Messages[0].Content[0].Type != "tool_use" || body.Messages[1].Role != "user" || body.Messages[1].Content[0].Type != "tool_result" || body.Messages[1].Content[0].ToolUseID != "toolu_1" || !body.Messages[1].Content[0].IsError {
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
func TestAnthropicAppliesProviderOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["max_tokens"] != float64(222) || body["service_tier"] != "auto" {
			t.Fatalf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	model := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("claude-test")
	stream, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options:  sdk.ModelOptions{MaxOutputTokens: 222, ProviderOptions: sdk.ProviderOptions{"anthropic": json.RawMessage(`{"service_tier":"auto"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestAnthropicProviderOptionsCannotOverrideCanonicalFields(t *testing.T) {
	model := NewProvider(ProviderOptions{BaseURL: "http://127.0.0.1"}).Model("claude-test")
	_, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options:  sdk.ModelOptions{ProviderOptions: sdk.ProviderOptions{"anthropic": json.RawMessage(`{"max_tokens":1}`)}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot override canonical request field") {
		t.Fatalf("error = %v", err)
	}
}

func TestAnthropicCapabilities(t *testing.T) {
	model := NewProvider(ProviderOptions{}).Model("claude-test")
	caps := model.Capabilities()
	if !caps.Streaming || !caps.Tools || !caps.Vision || !caps.ProviderOptions || !caps.RawChunks {
		t.Fatalf("Capabilities() = %#v", caps)
	}
	if !caps.ToolResultErrors {
		t.Fatalf("Capabilities() = %#v, want tool result errors", caps)
	}
}

func TestAnthropicAppliesToolProviderOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		tools, _ := body["tools"].([]any)
		toolObj, _ := tools[0].(map[string]any)
		cache, _ := toolObj["cache_control"].(map[string]any)
		if cache["type"] != "ephemeral" {
			t.Fatalf("tool = %#v, want cache_control", toolObj)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()
	model := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("claude-test")
	stream, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Tools:    []sdk.Tool{{Name: "lookup", Description: "lookup", ProviderOptions: sdk.ProviderOptions{"anthropic": json.RawMessage(`{"cache_control":{"type":"ephemeral"}}`)}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestAnthropicToolProviderOptionsCannotOverrideCanonicalFields(t *testing.T) {
	model := NewProvider(ProviderOptions{BaseURL: "http://127.0.0.1"}).Model("claude-test")
	_, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Tools:    []sdk.Tool{{Name: "lookup", Description: "lookup", ProviderOptions: sdk.ProviderOptions{"anthropic": json.RawMessage(`{"name":"other"}`)}}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot override canonical") {
		t.Fatalf("error = %v, want protected tool field error", err)
	}
}

func TestAnthropicIncludesRawChunksOnRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()
	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("claude-test").Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options:  sdk.ModelOptions{IncludeRawChunks: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	var events []sdk.Event
	for {
		event, err := stream.Next(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if len(events) != 4 || events[0].Kind != sdk.EventRaw || events[1].Kind != sdk.EventUsage || events[2].Kind != sdk.EventRaw || events[3].Kind != sdk.EventFinish {
		t.Fatalf("events = %#v", events)
	}
	if !strings.Contains(string(events[0].RawData), `"message_start"`) {
		t.Fatalf("raw = %q", events[0].RawData)
	}
}

func TestBuildRequestRequiresInitialToolUse(t *testing.T) {
	body, err := buildRequest("claude-test", sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "inspect"}},
		Tools:    []sdk.Tool{{Name: "read_file", Description: "read file", InputSchema: map[string]any{"type": "object"}}},
		Options:  sdk.ModelOptions{ToolChoice: sdk.ToolChoiceRequired},
	}, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if body.ToolChoice == nil || body.ToolChoice.Type != "any" {
		t.Fatalf("tool choice = %#v, want any", body.ToolChoice)
	}
}

func TestAnthropicEncodesAdaptiveReasoningEffort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		thinking, _ := body["thinking"].(map[string]any)
		output, _ := body["output_config"].(map[string]any)
		if thinking["type"] != "adaptive" || output["effort"] != "high" {
			t.Fatalf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
	}))
	defer server.Close()

	model := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("claude-opus-5")
	stream, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options:  sdk.ModelOptions{ReasoningEffort: sdk.ReasoningHigh},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestAnthropicRejectsReasoningNone(t *testing.T) {
	model := NewProvider(ProviderOptions{BaseURL: "http://127.0.0.1"}).Model("claude-opus-5")
	_, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options:  sdk.ModelOptions{ReasoningEffort: sdk.ReasoningNone},
	})
	if err == nil || !strings.Contains(err.Error(), "does not support reasoning effort") {
		t.Fatalf("error = %v", err)
	}
}
