package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func collectEvents(t *testing.T, stream sdk.Stream) []sdk.Event {
	t.Helper()
	defer stream.Close()
	var events []sdk.Event
	for {
		event, err := stream.Next(context.Background())
		if err == io.EOF {
			return events
		}
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		events = append(events, event)
	}
}

func TestChatStreamTextAndHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("X-Test"); got != "agent" {
			t.Fatalf("X-Test = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "Proton-Test" {
			t.Fatalf("User-Agent = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	provider := NewProvider(ProviderOptions{BaseURL: server.URL + "/v1", APIKey: "secret", UserAgent: "Proton-Test", Headers: http.Header{"X-Test": []string{"agent"}}})
	stream, err := provider.Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	events := collectEvents(t, stream)
	if len(events) != 2 || events[0].Kind != sdk.EventTextDelta || events[0].Text != "hello" || events[1].Kind != sdk.EventFinish || events[1].FinishReason != sdk.FinishStop {
		t.Fatalf("events = %#v", events)
	}
}

func TestChatStreamToolLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"read_file\",\"arguments\":\"{\\\"path\\\":\"}}]},\"finish_reason\":null}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"README.md\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "inspect"}}, Tools: []sdk.Tool{{Name: "read_file", Description: "read a file", InputSchema: map[string]any{"type": "object"}}}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	events := collectEvents(t, stream)
	var complete *sdk.ToolCall
	var sawStart, sawDelta bool
	for _, event := range events {
		switch event.Kind {
		case sdk.EventToolCallStart:
			sawStart = true
		case sdk.EventToolCallDelta:
			sawDelta = true
		case sdk.EventToolCall:
			call := event.ToolCall
			complete = &call
		}
	}
	if !sawStart || !sawDelta || complete == nil {
		t.Fatalf("tool lifecycle events = %#v", events)
	}
	if complete.ID != "call-1" || complete.Name != "read_file" || string(complete.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("complete tool call = %#v", complete)
	}
}

func TestResponsesAPIRequestAndStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "response-model" {
			t.Fatalf("model = %#v", body["model"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"answer\"}\n\n")
		io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":3,\"output_tokens\":1,\"total_tokens\":4}}}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL + "/v1"}).Model("response-model", WithResponsesAPI()).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	events := collectEvents(t, stream)
	joined := make([]string, 0, len(events))
	var usage sdk.Usage
	for _, event := range events {
		joined = append(joined, string(event.Kind))
		if event.Kind == sdk.EventUsage {
			usage = event.Usage
		}
	}
	if !strings.Contains(strings.Join(joined, ","), "text_delta,usage,finish") {
		t.Fatalf("events = %#v", events)
	}
	if usage.TotalTokens != 4 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestOpenAIRetriesTransientStatus(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	provider := NewProvider(ProviderOptions{BaseURL: server.URL, MaxRetries: 1, RetryBackoff: time.Millisecond})
	stream, err := provider.Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_ = collectEvents(t, stream)
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestOpenAIReportsIncompleteStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	for {
		_, err = stream.Next(context.Background())
		if err != nil {
			break
		}
	}
	if !errors.Is(err, sdk.ErrIncompleteStream) {
		t.Fatalf("error = %v, want ErrIncompleteStream", err)
	}
}

func TestOpenAIGeneratesFallbackToolCallID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"read_file\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "inspect"}}, Tools: []sdk.Tool{{Name: "read_file", Description: "read file"}}})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, stream)
	for _, event := range events {
		if event.Kind == sdk.EventToolCall {
			if event.ToolCall.ID != "generated_call_1" {
				t.Fatalf("tool call id = %q", event.ToolCall.ID)
			}
			return
		}
	}
	t.Fatal("missing tool call event")
}
func TestOpenAIHTTPErrorIsNormalized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limit_exceeded"}}`))
	}))
	defer server.Close()

	model := NewProvider(ProviderOptions{BaseURL: server.URL, MaxRetries: 0}).Model("test-model")
	_, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err == nil {
		t.Fatal("expected error")
	}
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %T, want *sdk.ProviderError", err)
	}
	if providerErr.Kind != sdk.ErrorRateLimit || providerErr.StatusCode != http.StatusTooManyRequests || !providerErr.Retryable {
		t.Fatalf("provider error = %#v", providerErr)
	}
}
