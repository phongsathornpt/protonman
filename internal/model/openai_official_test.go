package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/projectTHORN/proton/internal/tool"
)

func TestOfficialOpenAIResponsesStreamText(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("request path = %q, want /responses", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("x-session-affinity"); got != "session-1" {
			t.Errorf("session affinity = %q, want session-1", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w,
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n",
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\" world\"}\n\n",
			"data: {\"type\":\"response.completed\"}\n\n",
		)
	}))
	defer server.Close()

	client := NewOfficialOpenAIClient(
		server.URL,
		"test-key",
		"responses-model",
		WithSessionID("session-1"),
	)
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	if got := requestBody["model"]; got != "responses-model" {
		t.Fatalf("model = %#v, want responses-model", got)
	}
	if got := requestBody["tool_choice"]; got != "none" {
		t.Fatalf("tool_choice = %#v, want none", got)
	}
	if _, ok := requestBody["tools"]; ok {
		t.Fatalf("request unexpectedly contains tools: %#v", requestBody["tools"])
	}

	first, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 1 error = %v", err)
	}
	second, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() 2 error = %v", err)
	}
	done, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() done error = %v", err)
	}
	if first.Kind != EventTextDelta || first.Text != "hello" {
		t.Fatalf("first event = %+v", first)
	}
	if second.Kind != EventTextDelta || second.Text != " world" {
		t.Fatalf("second event = %+v", second)
	}
	if done.Kind != EventDone {
		t.Fatalf("done event = %+v", done)
	}
}

func TestOfficialOpenAIResponsesStreamToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w,
			"data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"item-1\",\"type\":\"function_call\",\"name\":\"read_file\",\"call_id\":\"call-1\"}}\n\n",
			"data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"item-1\",\"delta\":\"{\\\"path\\\":\\\"README.md\\\"}\"}\n\n",
			"data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"item-1\",\"type\":\"function_call\",\"name\":\"read_file\",\"call_id\":\"call-1\",\"arguments\":\"{\\\"path\\\":\\\"README.md\\\"}\"}}\n\n",
			"data: {\"type\":\"response.completed\"}\n\n",
		)
	}))
	defer server.Close()

	client := NewOfficialOpenAIClient(server.URL, "key", "responses-model")
	stream, err := client.Stream(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "read"}},
		Tools: []tool.Definition{{
			Name:        "read_file",
			Description: "read a file",
			Kind:        tool.KindRead,
		}},
	})
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer stream.Close()

	event, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() tool call error = %v", err)
	}
	if event.Kind != EventToolCall {
		t.Fatalf("event kind = %s, want tool_call", event.Kind)
	}
	if event.ToolCall.ID != "call-1" || event.ToolCall.Name != "read_file" {
		t.Fatalf("tool call = %+v", event.ToolCall)
	}
	if got, want := string(event.ToolCall.Arguments), `{"path":"README.md"}`; got != want {
		t.Fatalf("arguments = %s, want %s", got, want)
	}

	done, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() done error = %v", err)
	}
	if done.Kind != EventDone {
		t.Fatalf("done event = %+v", done)
	}
}

func TestOfficialResponsesStreamRejectsMissingCallID(t *testing.T) {
	stream := newOfficialResponsesStream(&officialResponsesSourceStub{
		events: []responses.ResponseStreamEventUnion{
			{
				Type: "response.output_item.done",
				Item: responses.ResponseOutputItemUnion{
					ID:        "item-1",
					Type:      "function_call",
					Name:      "read_file",
					Arguments: responses.ResponseOutputItemUnionArguments{OfString: "{}"},
				},
			},
		},
	})

	_, err := stream.Next(context.Background())
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("Next() error = %v, want invalid event", err)
	}
}

func TestOfficialResponsesStreamReportsIncompleteEOF(t *testing.T) {
	stream := newOfficialResponsesStream(&officialResponsesSourceStub{
		events: []responses.ResponseStreamEventUnion{{
			Type:  "response.output_text.delta",
			Delta: "partial",
		}},
	})

	event, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("Next() text error = %v", err)
	}
	if event.Text != "partial" {
		t.Fatalf("text event = %+v", event)
	}
	_, err = stream.Next(context.Background())
	if !errors.Is(err, ErrIncompleteStream) {
		t.Fatalf("Next() error = %v, want incomplete stream", err)
	}
}

type officialResponsesSourceStub struct {
	events  []responses.ResponseStreamEventUnion
	index   int
	current responses.ResponseStreamEventUnion
	err     error
}

func (s *officialResponsesSourceStub) Next() bool {
	if s.index >= len(s.events) {
		return false
	}
	s.current = s.events[s.index]
	s.index++
	return true
}

func (s *officialResponsesSourceStub) Current() responses.ResponseStreamEventUnion {
	return s.current
}

func (s *officialResponsesSourceStub) Err() error {
	return s.err
}

func (s *officialResponsesSourceStub) Close() error {
	return nil
}
