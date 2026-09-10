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

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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
		if got := r.Header.Get("User-Agent"); got != "Protonman-Test" {
			t.Fatalf("User-Agent = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Request-Id", "req-openai-1")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	provider := NewProvider(ProviderOptions{BaseURL: server.URL + "/v1", APIKey: "secret", UserAgent: "Protonman-Test", Headers: http.Header{"X-Test": []string{"agent"}}})
	stream, err := provider.Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	events := collectEvents(t, stream)
	if len(events) != 2 || events[0].Kind != sdk.EventTextDelta || events[0].Text != "hello" || events[1].Kind != sdk.EventFinish || events[1].FinishReason != sdk.FinishStop {
		t.Fatalf("events = %#v", events)
	}
	if got := string(events[1].ProviderMetadata["openai"]); !strings.Contains(got, `"request_id":"req-openai-1"`) {
		t.Fatalf("provider metadata = %s", got)
	}
}

func TestChatStreamToolLifecycle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\"}}]},\"finish_reason\":null}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"\\\"README.md\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "inspect"}}, Tools: []sdk.Tool{{Name: "read", Description: "read a file", InputSchema: map[string]any{"type": "object"}}}})
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
	if complete.ID != "call-1" || complete.Name != "read" || string(complete.Arguments) != `{"path":"README.md"}` {
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

func TestOpenAISessionIDHeaderPersistsAcrossRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if got := r.Header.Get("X-Session-Id"); got != "session-123" {
			t.Fatalf("attempt %d X-Session-Id = %q", attempts, got)
		}
		if attempts == 1 {
			http.Error(w, "temporary", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	model := NewProvider(ProviderOptions{
		BaseURL: server.URL, MaxRetries: 1, RetryBackoff: time.Millisecond,
		Headers: http.Header{"X-Session-Id": []string{"wrong-session"}},
	}).Model("test-model")
	stream, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Metadata: sdk.RequestMetadata{SessionID: " session-123 "},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
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
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"read\",\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "inspect"}}, Tools: []sdk.Tool{{Name: "read", Description: "read file"}}})
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
func TestOpenAIStreamErrorIsNormalized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"error\":{\"message\":\"slow down\",\"type\":\"rate_limit_error\",\"code\":\"rate_limit_exceeded\"}}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, err = stream.Next(context.Background())
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != sdk.ErrorRateLimit || providerErr.Code != "rate_limit_exceeded" {
		t.Fatalf("stream error = %#v (%v)", providerErr, err)
	}
}
func TestOpenAIChatAppliesModelAndProviderOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["max_tokens"] != float64(123) || body["reasoning_effort"] != "high" || body["service_tier"] != "auto" {
			t.Fatalf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	model := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model")
	stream, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options: sdk.ModelOptions{
			MaxOutputTokens: 123, ReasoningEffort: sdk.ReasoningHigh,
			ProviderOptions: sdk.ProviderOptions{"openai": json.RawMessage(`{"service_tier":"auto"}`)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestOpenAIResponsesEncodesReasoningEffort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		reasoning, _ := body["reasoning"].(map[string]any)
		if reasoning["effort"] != "xhigh" {
			t.Fatalf("body = %#v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{}}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("response-model", WithResponsesAPI()).Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options:  sdk.ModelOptions{ReasoningEffort: sdk.ReasoningXHigh},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestOpenAIProviderOptionsCannotOverrideReasoning(t *testing.T) {
	model := NewProvider(ProviderOptions{BaseURL: "http://127.0.0.1"}).Model("test-model")
	_, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options: sdk.ModelOptions{
			ReasoningEffort: sdk.ReasoningHigh,
			ProviderOptions: sdk.ProviderOptions{"openai": json.RawMessage(`{"reasoning_effort":"low"}`)},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot override canonical request field") {
		t.Fatalf("error = %v", err)
	}
}

func TestOpenAIProviderOptionsCannotOverrideCanonicalFields(t *testing.T) {
	model := NewProvider(ProviderOptions{BaseURL: "http://127.0.0.1"}).Model("test-model")
	_, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options:  sdk.ModelOptions{ProviderOptions: sdk.ProviderOptions{"openai": json.RawMessage(`{"model":"other"}`)}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot override canonical request field") {
		t.Fatalf("error = %v", err)
	}
}

func TestOpenAICapabilities(t *testing.T) {
	model := NewProvider(ProviderOptions{}).Model("test-model")
	caps := model.Capabilities()
	if !caps.Streaming || !caps.Tools || !caps.Vision || !caps.ProviderOptions || !caps.RawChunks {
		t.Fatalf("Capabilities() = %#v", caps)
	}
	if caps.ToolResultErrors {
		t.Fatalf("Capabilities() = %#v, unexpected tool result errors", caps)
	}
}

func TestOpenAIChatAppliesToolProviderOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		tools, _ := body["tools"].([]any)
		toolObj, _ := tools[0].(map[string]any)
		function, _ := toolObj["function"].(map[string]any)
		if strict, _ := function["strict"].(bool); !strict {
			t.Fatalf("function = %#v, want strict=true", function)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model")
	stream, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Tools:    []sdk.Tool{{Name: "lookup", Description: "lookup", ProviderOptions: sdk.ProviderOptions{"openai": json.RawMessage(`{"strict":true}`)}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestOpenAIToolProviderOptionsCannotOverrideCanonicalFields(t *testing.T) {
	model := NewProvider(ProviderOptions{BaseURL: "http://127.0.0.1"}).Model("test-model")
	_, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Tools:    []sdk.Tool{{Name: "lookup", Description: "lookup", ProviderOptions: sdk.ProviderOptions{"openai": json.RawMessage(`{"name":"other"}`)}}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot override canonical") {
		t.Fatalf("error = %v, want protected tool field error", err)
	}
}

func TestOpenAIIncludesRawChunksOnRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":null}]}\n\n")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Options:  sdk.ModelOptions{IncludeRawChunks: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, stream)
	if len(events) != 4 || events[0].Kind != sdk.EventRaw || events[1].Kind != sdk.EventTextDelta || events[2].Kind != sdk.EventRaw || events[3].Kind != sdk.EventFinish {
		t.Fatalf("events = %#v", events)
	}
	if !strings.Contains(string(events[0].RawData), `"content":"hello"`) {
		t.Fatalf("raw = %q", events[0].RawData)
	}
}

func TestToolChoiceRequired(t *testing.T) {
	if got := toolChoice(1, sdk.ToolChoiceRequired); got != "required" {
		t.Fatalf("toolChoice required = %q", got)
	}
	if got := toolChoice(1, sdk.ToolChoiceAuto); got != "" {
		t.Fatalf("toolChoice auto = %q", got)
	}
	if got := toolChoice(0, sdk.ToolChoiceRequired); got != "none" {
		t.Fatalf("toolChoice without tools = %q", got)
	}
}

func TestOpenCodeFreeUsageLimitIsStructuredAndNotRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"type":"FreeUsageLimitError","message":"Rate limit exceeded. Please try again later."}}`)
	}))
	defer server.Close()
	model := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL, MaxRetries: 2}).Model("free-model")
	_, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) || providerErr.RateLimit == nil {
		t.Fatalf("error = %#v (%v)", providerErr, err)
	}
	if providerErr.Provider != "opencode" || providerErr.RateLimit.Kind != sdk.RateLimitFreeUsage || providerErr.Retryable {
		t.Fatalf("provider error = %#v", providerErr)
	}
	if providerErr.RateLimit.RetryAfter != time.Hour {
		t.Fatalf("retry after = %s", providerErr.RateLimit.RetryAfter)
	}
}

func TestOpenCodeGoUsageLimitClassifiesWindow(t *testing.T) {
	body := []byte(`{"error":{"type":"GoUsageLimitError","message":"usage exhausted"},"metadata":{"limitName":"weekly"}}`)
	err := providerError("opencode", http.StatusTooManyRequests, body, nil)
	if err.RateLimit == nil || err.RateLimit.Kind != sdk.RateLimitGoWeekly || err.RateLimit.Scope != sdk.RateLimitScopeAccount || err.Retryable {
		t.Fatalf("provider error = %#v", err)
	}
}

func TestOpenCodeProviderRateLimitRemainsRetryable(t *testing.T) {
	body := []byte(`{"error":{"type":"rate_limit_error","message":"Provider rate limit exceeded"}}`)
	err := providerError("opencode", http.StatusTooManyRequests, body, http.Header{"Retry-After": []string{"2"}})
	if err.RateLimit == nil || err.RateLimit.Kind != sdk.RateLimitProvider || err.RateLimit.Scope != sdk.RateLimitScopeProvider || !err.Retryable {
		t.Fatalf("provider error = %#v", err)
	}
}

func TestOpenCodeProviderIdentityIsPreserved(t *testing.T) {
	model := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: "https://example.test/v1"}).Model("test")
	if model.Provider() != "opencode" {
		t.Fatalf("provider = %q", model.Provider())
	}
}

func TestOpenCodeFreeUsageLimitDoesNotConsumeRetryBudget(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"type":"FreeUsageLimitError","message":"quota exhausted"}}`)
	}))
	defer server.Close()
	model := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL, MaxRetries: 3, RetryBackoff: time.Millisecond}).Model("free")
	_, _ = model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestOpenCodeTransientRateLimitRespectsShortReset(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("X-RateLimit-Reset", "5ms")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"slow down"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL, MaxRetries: 2, RetryBackoff: time.Second}).Model("test")
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestOpenCodeLongRetryAfterReturnsImmediately(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"slow down"}}`)
	}))
	defer server.Close()
	model := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL, MaxRetries: 3, MaxRetryAfter: 10 * time.Millisecond}).Model("test")
	_, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err == nil || attempts != 1 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
}

func TestOpenCodeGoUsageLimitWindows(t *testing.T) {
	for _, tc := range []struct {
		name string
		want sdk.RateLimitKind
	}{
		{"5 hour", sdk.RateLimitGoFiveHour},
		{"weekly", sdk.RateLimitGoWeekly},
		{"monthly", sdk.RateLimitGoMonthly},
		{"daily", sdk.RateLimitUnknownQuota},
	} {
		body := []byte(`{"error":{"type":"GoUsageLimitError","message":"usage exhausted"},"metadata":{"limitName":"` + tc.name + `"}}`)
		err := providerError("opencode", http.StatusTooManyRequests, body, nil)
		if err.RateLimit == nil || err.RateLimit.Kind != tc.want || err.Retryable {
			t.Fatalf("limit %q = %#v", tc.name, err)
		}
	}
}

func TestOpenCodeStreamGoUsageLimitPreservesMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"error\":{\"type\":\"GoUsageLimitError\",\"message\":\"usage exhausted\",\"metadata\":{\"limitName\":\"monthly\"}}}\n\n")
	}))
	defer server.Close()
	stream, err := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL}).Model("test").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, err = stream.Next(context.Background())
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) || providerErr.RateLimit == nil || providerErr.RateLimit.Kind != sdk.RateLimitGoMonthly {
		t.Fatalf("stream error = %#v (%v)", providerErr, err)
	}
}

func TestOpenCodeSuccessfulResponsePublishesRateLimitMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "42")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	stream, err := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL}).Model("test").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	events := collectEvents(t, stream)
	raw := string(events[len(events)-1].ProviderMetadata["opencode"])
	if !strings.Contains(raw, `"rate_limit"`) || !strings.Contains(raw, `"remaining":42`) {
		t.Fatalf("metadata = %s", raw)
	}
}

func TestOpenCodeFreeUsageLimitOverridesHTTP403(t *testing.T) {
	err := providerError("opencode", http.StatusForbidden, []byte(`{"error":{"type":"FreeUsageLimitError","message":"free quota exhausted"}}`), nil)
	if err.Kind != sdk.ErrorRateLimit || err.RateLimit == nil || err.RateLimit.Kind != sdk.RateLimitFreeUsage || err.Retryable {
		t.Fatalf("provider error = %#v", err)
	}
}

func TestOpenCodeMalformedRetryAfterFallsBackToBackoff(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "eventually")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"slow down"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL, MaxRetries: 1, RetryBackoff: time.Millisecond}).Model("test")
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestOpenCodeCancellationInterruptsRateLimitWait(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Reset", "5s")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"slow down"}}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	model := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL, MaxRetries: 1, MaxRetryAfter: 10 * time.Second}).Model("test")
	_, err := model.Stream(ctx, sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestOpenCodeProviderLimitRetriesWithinHorizon(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("X-RateLimit-Reset", "2ms")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"Provider rate limit exceeded"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := NewProvider(ProviderOptions{ProviderName: "opencode", BaseURL: server.URL, MaxRetries: 1}).Model("test")
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestQwenHybridDashScopeDisablesThinkingNatively(t *testing.T) {
	model := NewProvider(ProviderOptions{BaseURL: "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"}).Model("qwen3.6-plus")
	effort, enabled := model.qwenChatReasoning(sdk.ReasoningNone)
	if effort != sdk.ReasoningDefault || enabled == nil || *enabled {
		t.Fatalf("qwen hybrid mapping = effort %q enabled %#v", effort, enabled)
	}
}

func TestQwen38MaxKeepsReasoningEffort(t *testing.T) {
	model := NewProvider(ProviderOptions{BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1"}).Model("qwen3.8-max-latest")
	effort, enabled := model.qwenChatReasoning(sdk.ReasoningXHigh)
	if effort != sdk.ReasoningXHigh || enabled != nil {
		t.Fatalf("qwen3.8 max mapping = effort %q enabled %#v", effort, enabled)
	}
}

func TestQwenHybridGatewayDoesNotRewriteReasoning(t *testing.T) {
	model := NewProvider(ProviderOptions{BaseURL: "https://protonman.dev/api/v1"}).Model("qwen3.6-plus")
	effort, enabled := model.qwenChatReasoning(sdk.ReasoningNone)
	if effort != sdk.ReasoningNone || enabled != nil {
		t.Fatalf("gateway mapping = effort %q enabled %#v", effort, enabled)
	}
}

func TestChatAssistantTextAndToolCallsRemainOneMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []chatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Messages) != 1 {
			t.Fatalf("messages = %#v, want one assistant message", body.Messages)
		}
		msg := body.Messages[0]
		if msg.Role != "assistant" || len(msg.ToolCalls) != 1 {
			t.Fatalf("message = %#v", msg)
		}
		content, ok := msg.Content.(string)
		if !ok || content != "I'll inspect it." {
			t.Fatalf("content = %#v", msg.Content)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("test-model").Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{
		Role: sdk.RoleAssistant, Content: "I'll inspect it.", ToolCalls: []sdk.ToolCall{{ID: "call_1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestResponsesFunctionCallNormalizesFinishReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"item_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"read\"}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"item_1\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"read\",\"arguments\":\"{}\"}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{}}\n\n")
	}))
	defer server.Close()
	stream, err := NewProvider(ProviderOptions{BaseURL: server.URL}).Model("response-model", WithResponsesAPI()).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.FinishReason != sdk.FinishToolCalls || len(result.ToolCalls) != 1 {
		t.Fatalf("result = %#v", result)
	}
}
