package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/tool"
)

func TestOpenAIClientStreamText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			http.Error(w, "invalid accept", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world!\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewOpenAIClient(ts.URL, "test-key", "test-model")
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	// 1. Text delta "Hello"
	ev1, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 1 error = %v", err)
	}
	if ev1.Kind != EventTextDelta || ev1.Text != "Hello" {
		t.Fatalf("unexpected ev1: %+v", ev1)
	}

	// 2. Text delta " world!"
	ev2, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 2 error = %v", err)
	}
	if ev2.Kind != EventTextDelta || ev2.Text != " world!" {
		t.Fatalf("unexpected ev2: %+v", ev2)
	}

	// 3. EventDone
	ev3, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 3 error = %v", err)
	}
	if ev3.Kind != EventDone {
		t.Fatalf("unexpected ev3 kind: %s", ev3.Kind)
	}
}

func TestOpenAIClientStreamToolCalls(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		// Chunk 1: tool call id & name
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_abc\",\"type\":\"function\",\"function\":{\"name\":\"read_file\",\"arguments\":\"\"}}]}}]}\n\n")
		flusher.Flush()
		// Chunk 2: arguments part 1
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"path\\\":\\\"\"}}]}}]}\n\n")
		flusher.Flush()
		// Chunk 3: arguments part 2
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"README.md\\\"}\"}}]}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewOpenAIClient(ts.URL, "key", "model")
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{
			{Role: RoleUser, Content: "read readme"},
		},
		Tools: []tool.Definition{
			{
				Name:        "read_file",
				Description: "reads a file",
				Kind:        tool.KindRead,
			},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	ev1, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if ev1.Kind != EventToolCall {
		t.Fatalf("expected EventToolCall, got: %s", ev1.Kind)
	}
	if ev1.ToolCall.ID != "call_abc" || ev1.ToolCall.Name != "read_file" {
		t.Fatalf("unexpected tool call: %+v", ev1.ToolCall)
	}
	var args map[string]string
	if err := json.Unmarshal(ev1.ToolCall.Arguments, &args); err != nil {
		t.Fatalf("failed to unmarshal arguments: %v", err)
	}
	if args["path"] != "README.md" {
		t.Fatalf("unexpected path arg: %v", args["path"])
	}

	evDone, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() done error = %v", err)
	}
	if evDone.Kind != EventDone {
		t.Fatalf("expected EventDone, got: %s", evDone.Kind)
	}
}

func TestOpenAIClientUnauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid api key", http.StatusUnauthorized)
	}))
	defer ts.Close()

	client := NewOpenAIClient(ts.URL, "bad-key", "model")
	_, err := client.Stream(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestOpenAIClientContextCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(1 * time.Second)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	client := NewOpenAIClient(ts.URL, "key", "model")
	stream, err := client.Stream(ctx, Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	cancel()
	_, err = stream.Next(ctx)
	if err == nil {
		t.Fatal("expected context canceled error")
	}
}

func TestOpenAIClientHeaders(t *testing.T) {
	var receivedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
	}))
	defer ts.Close()

	// 1. Test standard endpoint with session ID
	client := NewOpenAIClient(ts.URL, "key", "model", WithSessionID("sess-standard"))
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()
	_, _ = stream.Next(context.Background())

	if receivedHeaders.Get("User-Agent") != "Proton/1.0" {
		t.Errorf("expected User-Agent Proton/1.0, got %q", receivedHeaders.Get("User-Agent"))
	}
	if receivedHeaders.Get("x-session-affinity") != "sess-standard" {
		t.Errorf("expected x-session-affinity sess-standard, got %q", receivedHeaders.Get("x-session-affinity"))
	}
	if receivedHeaders.Get("X-Session-Id") != "sess-standard" {
		t.Errorf("expected X-Session-Id sess-standard, got %q", receivedHeaders.Get("X-Session-Id"))
	}
	if receivedHeaders.Get("x-opencode-session") != "" {
		t.Errorf("expected no x-opencode-session for standard endpoint, got %q", receivedHeaders.Get("x-opencode-session"))
	}

	// 2. Test OpenCode endpoint headers
	// We simulate OpenCode domain by setting baseURL with opencode.ai substring
	opencodeClient := NewOpenAIClient(ts.URL, "", "nemotron-3.5-lightning-free",
		WithSessionID("sess-opencode-123"),
		WithClientName("proton-test"),
	)
	// Force baseURL to include opencode.ai to test header injection logic
	opencodeClient.baseURL = "https://opencode.ai/zen/v1"
	// Create mock transport pointing to ts.URL
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://opencode.ai/zen/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Verify headers injected by client logic
	if opencodeClient.sessionID != "sess-opencode-123" {
		t.Errorf("expected sessionID sess-opencode-123, got %q", opencodeClient.sessionID)
	}

	// Verify live Stream headers when baseURL contains opencode.ai
	// Use custom roundtripper to reroute opencode.ai to test server
	opencodeClient.httpClient.Transport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		receivedHeaders = req.Header.Clone()
		req.URL.Scheme = "http"
		req.URL.Host = ts.Listener.Addr().String()
		return http.DefaultTransport.RoundTrip(req)
	})

	stream2, err := opencodeClient.Stream(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() opencode error = %v", err)
	}
	defer stream2.Close()
	_, _ = stream2.Next(context.Background())

	if receivedHeaders.Get("x-opencode-session") != "sess-opencode-123" {
		t.Errorf("expected x-opencode-session 'sess-opencode-123', got %q", receivedHeaders.Get("x-opencode-session"))
	}
	if receivedHeaders.Get("x-opencode-client") != "proton-test" {
		t.Errorf("expected x-opencode-client 'proton-test', got %q", receivedHeaders.Get("x-opencode-client"))
	}
	if receivedHeaders.Get("x-session-affinity") != "sess-opencode-123" {
		t.Errorf("expected x-session-affinity 'sess-opencode-123', got %q", receivedHeaders.Get("x-session-affinity"))
	}
	if receivedHeaders.Get("User-Agent") != "Proton/1.0" {
		t.Errorf("expected User-Agent 'Proton/1.0', got %q", receivedHeaders.Get("User-Agent"))
	}
	if receivedHeaders.Get("Authorization") != "" {
		t.Errorf("expected empty Authorization header for free opencode tier, got %q", receivedHeaders.Get("Authorization"))
	}
	_ = httpReq
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

