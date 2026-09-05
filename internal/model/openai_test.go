package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestOpenAIClientAssistantToolCallsSplitting(t *testing.T) {
	var capturedPayload []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedPayload, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
	}))
	defer ts.Close()

	client := NewOpenAIClient(ts.URL, "key", "model")

	// Input conversation:
	// 1. User says "list files"
	// 2. Assistant has BOTH text content and tool calls (Round 1 output)
	// 3. Tool results (Round 1 execution)
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{
			{
				Role:    RoleUser,
				Content: "list files",
			},
			{
				Role:    RoleAssistant,
				Content: "Let me check the files.",
				ToolCalls: []ToolCall{
					{
						ID:        "call_1",
						Name:      "list_dir",
						Arguments: json.RawMessage(`{"path":"."}`),
					},
				},
			},
			{
				Role:       RoleTool,
				ToolCallID: "call_1",
				Content:    `["file1.txt", "file2.txt"]`,
			},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	_, _ = stream.Next(context.Background())

	var reqBody struct {
		Messages []struct {
			Role       string              `json:"role"`
			Content    *string             `json:"content"`
			ToolCalls  []openAIToolCallReq `json:"tool_calls"`
			ToolCallID string              `json:"tool_call_id"`
		} `json:"messages"`
	}

	if err := json.Unmarshal(capturedPayload, &reqBody); err != nil {
		t.Fatalf("failed to unmarshal captured payload: %v\nPayload: %s", err, string(capturedPayload))
	}

	// Should have been split into 4 messages:
	// 1. User
	// 2. Assistant (text only, no tool_calls)
	// 3. Assistant (tool_calls only, content == nil/omitted)
	// 4. Tool
	if len(reqBody.Messages) != 4 {
		t.Fatalf("expected 4 messages after splitting, got %d: %s", len(reqBody.Messages), string(capturedPayload))
	}

	// Message 1: User
	if reqBody.Messages[0].Role != "user" || reqBody.Messages[0].Content == nil || *reqBody.Messages[0].Content != "list files" {
		t.Errorf("msg 0 mismatch: %+v", reqBody.Messages[0])
	}

	// Message 2: Assistant text
	if reqBody.Messages[1].Role != "assistant" || reqBody.Messages[1].Content == nil || *reqBody.Messages[1].Content != "Let me check the files." || len(reqBody.Messages[1].ToolCalls) > 0 {
		t.Errorf("msg 1 mismatch: %+v", reqBody.Messages[1])
	}

	// Message 3: Assistant tool_calls (Content MUST be nil)
	if reqBody.Messages[2].Role != "assistant" || reqBody.Messages[2].Content != nil || len(reqBody.Messages[2].ToolCalls) != 1 {
		t.Errorf("msg 2 mismatch: %+v", reqBody.Messages[2])
	}
	if reqBody.Messages[2].ToolCalls[0].ID != "call_1" || reqBody.Messages[2].ToolCalls[0].Function.Name != "list_dir" {
		t.Errorf("msg 2 tool call mismatch: %+v", reqBody.Messages[2].ToolCalls[0])
	}

	// Message 4: Tool response
	if reqBody.Messages[3].Role != "tool" || reqBody.Messages[3].ToolCallID != "call_1" || reqBody.Messages[3].Content == nil {
		t.Errorf("msg 3 mismatch: %+v", reqBody.Messages[3])
	}
}

func TestOpenAIClientFallbackToolCallID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		// Model chunk sends function name and args, but NO ID (empty id)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"test_tool\",\"arguments\":\"{}\"}}]}}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewOpenAIClient(ts.URL, "key", "model")
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "run"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	ev, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if ev.Kind != EventToolCall {
		t.Fatalf("expected EventToolCall, got %s", ev.Kind)
	}
	if ev.ToolCall.ID == "" {
		t.Fatalf("expected generated fallback tool call ID, got empty string")
	}
	if !strings.HasPrefix(ev.ToolCall.ID, "call_") {
		t.Errorf("expected fallback ID to start with 'call_', got %q", ev.ToolCall.ID)
	}
}

func TestOpenAIClientRetryTransientErrors(t *testing.T) {
	attempts := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			// Simulate transient 503 from upstream
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"error":{"type":"server_error","message":"Upstream request failed: Endpoint is unavailable."}}`))
			return
		}
		// Second attempt succeeds
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"success after retry\"}}]}\n\n")
		fmt.Fprintf(w, "data: [DONE]\n\n")
		w.(http.Flusher).Flush()
	}))
	defer ts.Close()

	client := NewOpenAIClient(ts.URL, "key", "model")
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Stream() should have recovered on retry, got error: %v", err)
	}
	defer stream.Close()

	ev, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() error = %v", err)
	}
	if ev.Kind != EventTextDelta || ev.Text != "success after retry" {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if attempts != 2 {
		t.Errorf("expected exactly 2 attempts, got %d", attempts)
	}
}

func TestLiveOpenCodeMultiTurn(t *testing.T) {
	if os.Getenv("RUN_LIVE_TESTS") != "1" {
		t.Skip("skipping live network test without RUN_LIVE_TESTS=1")
	}

	client := NewOpenAIClient(
		"https://opencode.ai/zen/v1",
		"",
		"nemotron-3.5-lightning-free",
		WithSessionID("sess-test-live-1"),
		WithClientName("proton"),
	)

	tools := []tool.Definition{
		{
			Name:        "list_dir",
			Description: "List files in directory",
			Kind:        tool.KindRead,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		},
	}

	// Turn 1
	req1 := Request{
		Messages: []Message{
			{
				Role:    RoleSystem,
				Content: "You are a coding assistant with tool call capabilities.",
			},
			{
				Role:    RoleUser,
				Content: "Please list the files in the current directory using list_dir.",
			},
		},
		Tools: tools,
	}

	stream1, err := client.Stream(context.Background(), req1)
	if err != nil {
		if strings.Contains(err.Error(), "503") || strings.Contains(err.Error(), "502") {
			t.Skipf("upstream opencode gateway is temporarily unavailable: %v", err)
		}
		t.Fatalf("Stream 1 failed: %v", err)
	}

	var assistantText string
	var toolCalls []ToolCall

	for {
		ev, err := stream1.Next(context.Background())
		if err != nil {
			t.Fatalf("Stream 1 Next error: %v", err)
		}
		if ev.Kind == EventDone {
			break
		}
		if ev.Kind == EventTextDelta {
			assistantText += ev.Text
		}
		if ev.Kind == EventToolCall {
			toolCalls = append(toolCalls, ev.ToolCall)
		}
	}
	stream1.Close()

	if len(toolCalls) == 0 {
		t.Log("Model did not call tool in turn 1, skipping turn 2")
		return
	}

	// Force non-empty assistant text if model didn't emit text in turn 1
	// to specifically verify the bug from the user screenshot:
	// "Let me start by exploring the current workspace..." + tool_calls
	if assistantText == "" {
		assistantText = "Let me start by exploring the current workspace to understand the project structure."
	}
	t.Logf("Turn 1 assistant text (with text + tool_calls): %q", assistantText)
	t.Logf("Turn 1 tool call: %+v", toolCalls[0])

	// Turn 2: Feed tool output back to model
	req2 := Request{
		Messages: []Message{
			{
				Role:    RoleSystem,
				Content: "You are a coding assistant with tool call capabilities.",
			},
			{
				Role:    RoleUser,
				Content: "Please list the files in the current directory using list_dir.",
			},
			{
				Role:      RoleAssistant,
				Content:   assistantText,
				ToolCalls: toolCalls,
			},
			{
				Role:       RoleTool,
				ToolCallID: toolCalls[0].ID,
				Content:    `["README.md", "go.mod", "main.go"]`,
			},
		},
		Tools: tools,
	}

	stream2, err := client.Stream(context.Background(), req2)
	if err != nil {
		t.Fatalf("Stream 2 failed (THIS WAS THE 503 BUG): %v", err)
	}

	var turn2Text string
	for {
		ev, err := stream2.Next(context.Background())
		if err != nil {
			t.Fatalf("Stream 2 Next error: %v", err)
		}
		if ev.Kind == EventDone {
			break
		}
		if ev.Kind == EventTextDelta {
			turn2Text += ev.Text
		}
	}
	stream2.Close()

	t.Logf("Turn 2 assistant response successfully received: %q", turn2Text)
}

func TestOpenAIClientResponsesStreamText(t *testing.T) {
	var requestedPath string
	var capturedPayload []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		var err error
		capturedPayload, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello from \"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Muse Spark!\"}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"type\":\"response.completed\"}\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	// Using modelID muse-spark-1.3-contributor-free should automatically route to /responses
	client := NewOpenAIClient(ts.URL, "key", "muse-spark-1.3-contributor-free")
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{
			{Role: RoleSystem, Content: "You are an assistant"},
			{Role: RoleUser, Content: "Hello"},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	// Verify route target
	if requestedPath != "/responses" {
		t.Errorf("expected requestedPath /responses, got %q", requestedPath)
	}

	// Verify request payload uses "input" array instead of "messages"
	var parsedReq struct {
		Model string `json:"model"`
		Input []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(capturedPayload, &parsedReq); err != nil {
		t.Fatalf("failed to unmarshal request payload: %v\nPayload: %s", err, string(capturedPayload))
	}
	if len(parsedReq.Input) != 2 {
		t.Fatalf("expected 2 items in input, got %d", len(parsedReq.Input))
	}
	if parsedReq.Input[0].Role != "system" || parsedReq.Input[0].Content != "You are an assistant" {
		t.Errorf("unexpected input[0]: %+v", parsedReq.Input[0])
	}
	if parsedReq.Input[1].Role != "user" || parsedReq.Input[1].Content != "Hello" {
		t.Errorf("unexpected input[1]: %+v", parsedReq.Input[1])
	}

	// Read events
	ev1, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 1 error: %v", err)
	}
	if ev1.Kind != EventTextDelta || ev1.Text != "Hello from " {
		t.Errorf("unexpected ev1: %+v", ev1)
	}

	ev2, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 2 error: %v", err)
	}
	if ev2.Kind != EventTextDelta || ev2.Text != "Muse Spark!" {
		t.Errorf("unexpected ev2: %+v", ev2)
	}

	ev3, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 3 error: %v", err)
	}
	if ev3.Kind != EventDone {
		t.Errorf("expected EventDone, got %s", ev3.Kind)
	}
}

func TestOpenAIClientResponsesStreamToolCalls(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		// 1. Output item added (function call declaration)
		fmt.Fprintf(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"fc_100\",\"type\":\"function_call\",\"name\":\"list_dir\",\"call_id\":\"call_100\",\"arguments\":\"\"}}\n\n")
		flusher.Flush()
		// 2. Arguments delta
		fmt.Fprintf(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_100\",\"delta\":\"{\\\"path\\\":\\\"/tmp\\\"}\"}\n\n")
		flusher.Flush()
		// 3. Output item done
		fmt.Fprintf(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"fc_100\",\"type\":\"function_call\",\"name\":\"list_dir\",\"call_id\":\"call_100\",\"arguments\":\"{\\\"path\\\":\\\"/tmp\\\"}\"}}\n\n")
		flusher.Flush()
		// 4. Completed
		fmt.Fprintf(w, "data: {\"type\":\"response.completed\"}\n\n")
		flusher.Flush()
	}))
	defer ts.Close()

	client := NewOpenAIClient(ts.URL, "key", "muse-spark-1.3-contributor-free")
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "list"}},
		Tools: []tool.Definition{
			{Name: "list_dir", Description: "list files", Kind: tool.KindRead},
		},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	ev1, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 1 error: %v", err)
	}
	if ev1.Kind != EventToolCall {
		t.Fatalf("expected EventToolCall, got %s", ev1.Kind)
	}
	if ev1.ToolCall.ID != "call_100" || ev1.ToolCall.Name != "list_dir" {
		t.Errorf("unexpected tool call: %+v", ev1.ToolCall)
	}
	if string(ev1.ToolCall.Arguments) != `{"path":"/tmp"}` {
		t.Errorf("unexpected tool call arguments: %s", string(ev1.ToolCall.Arguments))
	}

	ev2, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 2 error: %v", err)
	}
	if ev2.Kind != EventDone {
		t.Errorf("expected EventDone, got %s", ev2.Kind)
	}
}

func TestLiveOpenCodeResponsesMuseSpark(t *testing.T) {
	if os.Getenv("RUN_LIVE_TESTS") != "1" {
		t.Skip("skipping live network test without RUN_LIVE_TESTS=1")
	}

	client := NewOpenAIClient(
		"https://opencode.ai/zen/v1",
		"",
		"muse-spark-1.3-contributor-free",
		WithSessionID("sess-test-live-muse"),
		WithClientName("proton"),
	)

	tools := []tool.Definition{
		{
			Name:        "list_dir",
			Description: "List files in directory",
			Kind:        tool.KindRead,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
			},
		},
	}

	req := Request{
		Messages: []Message{
			{
				Role:    RoleSystem,
				Content: "You are a helpful coding assistant with function call capabilities.",
			},
			{
				Role:    RoleUser,
				Content: "Please list the files in directory / using the list_dir tool.",
			},
		},
		Tools: tools,
	}

	stream, err := client.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Stream() failed for muse-spark on Responses endpoint: %v", err)
	}
	defer stream.Close()

	var textDelta string
	var toolCalls []ToolCall

	for {
		ev, err := stream.Next(context.Background())
		if err != nil {
			t.Fatalf("stream.Next() error: %v", err)
		}
		if ev.Kind == EventDone {
			break
		}
		if ev.Kind == EventTextDelta {
			textDelta += ev.Text
		}
		if ev.Kind == EventToolCall {
			toolCalls = append(toolCalls, ev.ToolCall)
		}
	}

	t.Logf("Muse Spark response text: %q", textDelta)
	t.Logf("Muse Spark tool calls received: %d", len(toolCalls))
	if len(toolCalls) > 0 {
		t.Logf("Tool call: %+v", toolCalls[0])
	}
}

func TestOpenAIStream_EOFAfterContentWithoutTrailingNewline(t *testing.T) {
	// The terminal marker can be the final line without a trailing newline.
	payload := "data: {\"choices\":[{\"delta\":{\"content\":\"Final chunk without newline\"}}]}\n" +
		"data: [DONE]"
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(payload)))
	defer stream.Close()

	var receivedText string
	gotDone := false

	for {
		ev, err := stream.Next(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}
		if ev.Kind == EventTextDelta {
			receivedText += ev.Text
		}
		if ev.Kind == EventDone {
			gotDone = true
		}
	}

	if receivedText != "Final chunk without newline" {
		t.Fatalf("receivedText = %q, want %q", receivedText, "Final chunk without newline")
	}
	if !gotDone {
		t.Fatal("expected EventDone before EOF")
	}
}

func TestOpenAIStream_ReportsIncompleteEOF(t *testing.T) {
	payload := `data: {"choices":[{"delta":{"content":"truncated response"}}]}`
	stream := newOpenAIStream(io.NopCloser(strings.NewReader(payload)))
	defer stream.Close()

	for {
		_, err := stream.Next(context.Background())
		if errors.Is(err, ErrIncompleteStream) {
			return
		}
		if errors.Is(err, io.EOF) {
			t.Fatal("stream returned EOF without reporting incomplete response")
		}
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}
	}
}

func TestOpenAIStreamRejectsMalformedSSEData(t *testing.T) {
	stream := newOpenAIStream(io.NopCloser(strings.NewReader("data: {not-json}\n\ndata: [DONE]\n")))
	defer stream.Close()

	_, err := stream.Next(context.Background())
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("Next() error = %v, want invalid model event", err)
	}
}
