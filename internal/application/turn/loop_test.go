package turn

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

func TestLoopStreamsTextAndCompletes(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []model.Event{
			{Kind: model.EventTextDelta, Text: "hello"},
			{Kind: model.EventTextDelta, Text: " world"},
			{Kind: model.EventDone},
		},
	}}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow)
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "say hello"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.Message.Content, "hello world"; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
	if got, want := result.Rounds, 1; got != want {
		t.Fatalf("rounds = %d, want %d", got, want)
	}
	if got, want := len(client.requests), 1; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	if got, want := len(client.requests[0].Tools), 1; got != want {
		t.Fatalf("published tools = %d, want %d", got, want)
	}
	gotKinds := eventKinds(events)
	wantKinds := []EventKind{EventTextDelta, EventTextDelta, EventCompleted}
	if !sameKinds(gotKinds, wantKinds) {
		t.Fatalf("events = %#v, want %#v", gotKinds, wantKinds)
	}
}

func TestLoopTranslatesToolCallsAndFeedsResultsBack(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []model.Event{
			{
				Kind: model.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"README.md"}`),
				},
			},
			{Kind: model.EventDone},
		}},
		{events: []model.Event{
			{Kind: model.EventTextDelta, Text: "I found it."},
			{Kind: model.EventDone},
		}},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow)
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read the README"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.Message.Content, "I found it."; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
	if got, want := len(handler.calls), 1; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if got, want := len(client.requests), 2; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	followUp := client.requests[1].Messages
	if got, want := len(followUp), 3; got != want {
		t.Fatalf("follow-up messages = %d, want %d", got, want)
	}
	if got, want := len(followUp[1].ToolCalls), 1; got != want {
		t.Fatalf("assistant tool calls = %d, want %d", got, want)
	}
	if got, want := followUp[1].ToolCalls[0].ID, "call-1"; got != want {
		t.Fatalf("assistant tool call ID = %q, want %q", got, want)
	}
	var toolMessage tool.Result
	if err := json.Unmarshal([]byte(followUp[2].Content), &toolMessage); err != nil {
		t.Fatalf("decode follow-up tool result: %v", err)
	}
	if got, want := toolMessage.Output, "file contents"; got != want {
		t.Fatalf("follow-up tool output = %q, want %q", got, want)
	}
	gotKinds := eventKinds(events)
	wantKinds := []EventKind{EventToolCall, EventToolResult, EventTextDelta, EventCompleted}
	if !sameKinds(gotKinds, wantKinds) {
		t.Fatalf("events = %#v, want %#v", gotKinds, wantKinds)
	}
}

func TestLoopKeepsPermissionDenialInsideToolConversation(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []model.Event{
			{
				Kind: model.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-denied",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":".env"}`),
				},
			},
			{Kind: model.EventDone},
		}},
		{events: []model.Event{
			{Kind: model.EventTextDelta, Text: "I cannot access that file."},
			{Kind: model.EventDone},
		}},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionDeny)
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read the secret"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := len(handler.calls), 0; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if got, want := result.Message.Content, "I cannot access that file."; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
	toolEvent := findEvent(events, EventToolResult)
	if toolEvent.Err == nil {
		t.Fatal("tool result error = nil, want permission denial")
	}
	if toolEvent.Result.Failure == nil || toolEvent.Result.Failure.Code != tool.ErrorCodePermissionDenied {
		t.Fatalf("tool result failure = %#v, want permission_denied", toolEvent.Result.Failure)
	}
	if got := client.requests[1].Messages[2].Content; !contains(got, string(tool.ErrorCodePermissionDenied)) {
		t.Fatalf("follow-up denial content = %q, want permission code", got)
	}
}

func TestLoopStopsAtMaxRounds(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []model.Event{
			{
				Kind: model.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-loop",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"README.md"}`),
				},
			},
			{Kind: model.EventDone},
		},
	}}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithMaxRounds(1))
	events := make([]Event, 0)

	_, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "keep going"}},
		collectEvents(&events),
	)
	if !errors.Is(err, ErrMaxRounds) {
		t.Fatalf("Run() error = %v, want ErrMaxRounds", err)
	}
	if got, want := len(client.requests), 1; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	if got := events[len(events)-1].Kind; got != EventFailed {
		t.Fatalf("last event kind = %q, want %q", got, EventFailed)
	}
}

type recordingHandler struct {
	definition tool.Definition
	calls      []tool.Call
}

func (h *recordingHandler) Definition() tool.Definition {
	return h.definition
}

func (h *recordingHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls = append(h.calls, call)
	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   "file contents",
	}, nil
}

type recordingRegistry struct {
	handler *recordingHandler
}

func (r *recordingRegistry) Lookup(name string) (tool.Handler, bool) {
	if name != r.handler.definition.Name {
		return nil, false
	}
	return r.handler, true
}

func (r *recordingRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.definition}
}

type scriptedStreamSpec struct {
	events   []model.Event
	closeErr error
}

type scriptedClient struct {
	streams  []scriptedStreamSpec
	requests []model.Request
}

func (c *scriptedClient) Stream(_ context.Context, request model.Request) (model.Stream, error) {
	if len(c.streams) == 0 {
		return nil, errors.New("no scripted model stream remains")
	}
	c.requests = append(c.requests, model.Request{
		Messages: model.CloneMessages(request.Messages),
		Tools:    append([]tool.Definition{}, request.Tools...),
	})
	spec := c.streams[0]
	c.streams = c.streams[1:]
	return &scriptedStream{events: spec.events, closeErr: spec.closeErr}, nil
}

type scriptedStream struct {
	events   []model.Event
	index    int
	closeErr error
}

func (s *scriptedStream) Next(ctx context.Context) (model.Event, error) {
	if err := ctx.Err(); err != nil {
		return model.Event{}, err
	}
	if s.index >= len(s.events) {
		return model.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *scriptedStream) Close() error {
	return s.closeErr
}

func newTestLoop(
	t *testing.T,
	client *scriptedClient,
	action permission.Action,
	options ...Option,
) (*Loop, *recordingHandler) {
	t.Helper()
	handler := &recordingHandler{
		definition: tool.Definition{
			Name:                "read_file",
			Description:         "read a file",
			Kind:                tool.KindRead,
			PermissionDetailKey: "path",
		},
	}
	policy, err := permission.NewPolicy(permission.Config{
		Rules: []permission.Rule{{
			Action: action,
			Tool:   permission.ToolRead,
		}},
	})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(
		&recordingRegistry{handler: handler},
		policy,
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	loop, err := NewLoop(client, service, options...)
	if err != nil {
		t.Fatalf("NewLoop() error = %v", err)
	}
	return loop, handler
}

func collectEvents(events *[]Event) Sink {
	return func(_ context.Context, event Event) error {
		*events = append(*events, event)
		return nil
	}
}

func eventKinds(events []Event) []EventKind {
	kinds := make([]EventKind, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func sameKinds(left []EventKind, right []EventKind) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func findEvent(events []Event, kind EventKind) Event {
	for _, event := range events {
		if event.Kind == kind {
			return event
		}
	}
	return Event{}
}

func contains(value string, target string) bool {
	return strings.Contains(value, target)
}
