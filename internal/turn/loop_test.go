package turn

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
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
	if got, want := len(result.Messages), 3; got != want {
		t.Fatalf("turn messages = %d, want %d", got, want)
	}
	if result.Messages[0].Role != model.RoleAssistant || len(result.Messages[0].ToolCalls) != 1 {
		t.Fatalf("first turn message = %#v, want assistant tool call", result.Messages[0])
	}
	if result.Messages[1].Role != model.RoleTool || result.Messages[1].ToolCallID != "call-1" {
		t.Fatalf("second turn message = %#v, want matching tool result", result.Messages[1])
	}
	if result.Messages[2].Role != model.RoleAssistant || result.Messages[2].Content != "I found it." {
		t.Fatalf("final turn message = %#v, want final assistant", result.Messages[2])
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

func TestLoopTimesOutIndividualToolCall(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []model.Event{
			{
				Kind: model.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-timeout",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"slow.txt"}`),
				},
			},
			{Kind: model.EventDone},
		}},
		{events: []model.Event{
			{Kind: model.EventTextDelta, Text: "timed out safely"},
			{Kind: model.EventDone},
		}},
	}}
	handler := &contextBlockingHandler{
		definition: readFileDefinition(),
		started:    make(chan struct{}),
	}
	loop := newLoopForHandler(
		t,
		client,
		handler,
		permission.ActionAllow,
		permission.ModeAsk,
		WithToolTimeout(20*time.Millisecond),
	)
	events := make([]Event, 0)
	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read slowly"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.Message.Content, "timed out safely"; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
	toolEvent := findEvent(events, EventToolResult)
	if toolEvent.Result.Failure == nil || toolEvent.Result.Failure.Code != tool.ErrorCodeDeadlineExceeded {
		t.Fatalf("tool result failure = %#v, want deadline_exceeded", toolEvent.Result.Failure)
	}
}

func TestLoopCancelsModelStreamWithParentContext(t *testing.T) {
	client := &blockingModelClient{started: make(chan struct{})}
	loop, _ := newTestLoop(t, client, permission.ActionAllow)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make([]Event, 0)
	resultCh := make(chan error, 1)
	go func() {
		_, err := loop.Run(
			ctx,
			[]model.Message{{Role: model.RoleUser, Content: "wait"}},
			collectEvents(&events),
		)
		resultCh <- err
	}()
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("model stream did not start")
	}
	cancel()
	select {
	case err := <-resultCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}
	if got := events[len(events)-1].Kind; got != EventFailed {
		t.Fatalf("last event kind = %q, want %q", got, EventFailed)
	}
}

func TestLoopRunsApprovedReadCallsWithBoundedConcurrency(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []model.Event{
			{
				Kind: model.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-read-1",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"one.txt"}`),
				},
			},
			{
				Kind: model.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-read-2",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"two.txt"}`),
				},
			},
			{Kind: model.EventDone},
		}},
		{events: []model.Event{
			{Kind: model.EventTextDelta, Text: "both read"},
			{Kind: model.EventDone},
		}},
	}}
	handler := &parallelHandler{
		definition: readFileDefinition(),
		started:    make(chan struct{}, 2),
		release:    make(chan struct{}),
	}
	loop := newLoopForHandler(
		t,
		client,
		handler,
		permission.ActionAllow,
		permission.ModeAlwaysApprove,
		WithMaxParallelReads(2),
	)
	resultCh := make(chan struct {
		result Result
		err    error
	}, 1)
	go func() {
		result, err := loop.Run(
			context.Background(),
			[]model.Message{{Role: model.RoleUser, Content: "read both"}},
			nil,
		)
		resultCh <- struct {
			result Result
			err    error
		}{result: result, err: err}
	}()
	for range 2 {
		select {
		case <-handler.started:
		case <-time.After(time.Second):
			t.Fatal("read calls did not start concurrently")
		}
	}
	close(handler.release)
	select {
	case outcome := <-resultCh:
		if outcome.err != nil {
			t.Fatalf("Run() error = %v", outcome.err)
		}
		if got, want := outcome.result.Message.Content, "both read"; got != want {
			t.Fatalf("final content = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not complete after read calls were released")
	}
	if got, want := handler.max.Load(), int32(2); got != want {
		t.Fatalf("maximum concurrent calls = %d, want %d", got, want)
	}
}

type recordingHandler struct {
	definition tool.Definition
	calls      []tool.Call
}

func readFileDefinition() tool.Definition {
	return tool.Definition{
		Name:                "read_file",
		Description:         "read a file",
		Kind:                tool.KindRead,
		PermissionDetailKey: "path",
	}
}

type contextBlockingHandler struct {
	definition tool.Definition
	started    chan struct{}
	once       sync.Once
}

func (h *contextBlockingHandler) Definition() tool.Definition {
	return h.definition
}

func (h *contextBlockingHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	h.once.Do(func() { close(h.started) })
	<-ctx.Done()
	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
	}, ctx.Err()
}

type parallelHandler struct {
	definition tool.Definition
	started    chan struct{}
	release    chan struct{}
	current    atomic.Int32
	max        atomic.Int32
}

func (h *parallelHandler) Definition() tool.Definition {
	return h.definition
}

func (h *parallelHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	current := h.current.Add(1)
	for {
		maximum := h.max.Load()
		if current <= maximum || h.max.CompareAndSwap(maximum, current) {
			break
		}
	}
	h.started <- struct{}{}
	select {
	case <-h.release:
	case <-ctx.Done():
	}
	h.current.Add(-1)
	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   "file contents",
	}, nil
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
	handler tool.Handler
}

func (r *recordingRegistry) Lookup(name string) (tool.Handler, bool) {
	if name != r.handler.Definition().Name {
		return nil, false
	}
	return r.handler, true
}

func (r *recordingRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.Definition()}
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
	client model.Client,
	action permission.Action,
	options ...Option,
) (*Loop, *recordingHandler) {
	t.Helper()
	handler := &recordingHandler{definition: readFileDefinition()}
	loop := newLoopForHandler(
		t,
		client,
		handler,
		action,
		permission.ModeAsk,
		options...,
	)
	return loop, handler
}

func newLoopForHandler(
	t *testing.T,
	client model.Client,
	handler tool.Handler,
	action permission.Action,
	mode permission.Mode,
	options ...Option,
) *Loop {
	t.Helper()
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
		toolcall.WithMode(mode),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	loop, err := NewLoop(client, service, options...)
	if err != nil {
		t.Fatalf("NewLoop() error = %v", err)
	}
	return loop
}

type blockingModelClient struct {
	started chan struct{}
	once    sync.Once
}

func (c *blockingModelClient) Stream(context.Context, model.Request) (model.Stream, error) {
	c.once.Do(func() { close(c.started) })
	return blockingModelStream{}, nil
}

type blockingModelStream struct{}

func (blockingModelStream) Next(ctx context.Context) (model.Event, error) {
	<-ctx.Done()
	return model.Event{}, ctx.Err()
}

func (blockingModelStream) Close() error {
	return nil
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

func TestLoopAugmentsSystemPromptWithSkillCatalog(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []model.Event{
			{Kind: model.EventTextDelta, Text: "I see the skills"},
			{Kind: model.EventDone},
		},
	}}}
	catalog := []skill.CatalogItem{
		{
			Name:        "pdf-processing",
			Description: "Handle PDFs",
			Location:    "/path/to/SKILL.md",
			Scope:       skill.ScopeUser,
		},
	}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithSkillCatalog(catalog))
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "help with pdf"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Message.Content != "I see the skills" {
		t.Fatalf("unexpected content: %q", result.Message.Content)
	}

	if len(client.requests) == 0 {
		t.Fatalf("expected at least 1 model request")
	}
	reqMessages := client.requests[0].Messages
	if len(reqMessages) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %d", len(reqMessages))
	}
	if reqMessages[0].Role != model.RoleSystem {
		t.Errorf("expected first message to be RoleSystem, got %s", reqMessages[0].Role)
	}
	if !strings.Contains(reqMessages[0].Content, "<available_skills>") || !strings.Contains(reqMessages[0].Content, "pdf-processing") {
		t.Errorf("system message missing skill catalog: %s", reqMessages[0].Content)
	}
}

func TestLoopDoesNotDuplicateSkillCatalogMarker(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []model.Event{{Kind: model.EventDone}},
	}}}
	catalog := []skill.CatalogItem{{
		Name:        "pdf-processing",
		Description: "Handle PDFs",
		Location:    "/path/to/SKILL.md",
		Scope:       skill.ScopeUser,
	}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithSkillCatalog(catalog))
	priorSystem := skillPromptMarker + "\n" + skill.SystemPromptSection(catalog)
	_, err := loop.Run(context.Background(), []model.Message{
		{Role: model.RoleSystem, Content: priorSystem},
		{Role: model.RoleUser, Content: "help with pdf"},
	}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	content := client.requests[0].Messages[0].Content
	if got := strings.Count(content, skillPromptMarker); got != 1 {
		t.Fatalf("skill marker count = %d, want 1: %q", got, content)
	}
	if got := strings.Count(content, "<available_skills>"); got != 1 {
		t.Fatalf("skill catalog count = %d, want 1: %q", got, content)
	}
}

func TestLoopDynamicActiveSkillsWithRegistry(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []model.Event{{Kind: model.EventDone}}},
		{events: []model.Event{{Kind: model.EventDone}}},
	}}

	s1 := skill.Skill{
		Name:         "active-skill",
		Description:  "An active skill",
		Location:     "/loc/active/SKILL.md",
		BaseDir:      "/loc/active",
		Scope:        skill.ScopeUser,
		Instructions: "# Active Instructions\nDo active stuff.",
	}
	s2 := skill.Skill{
		Name:         "available-skill",
		Description:  "An available skill",
		Location:     "/loc/avail/SKILL.md",
		BaseDir:      "/loc/avail",
		Scope:        skill.ScopeProject,
		Instructions: "# Avail Instructions",
	}

	reg := skill.NewRegistry(s1, s2)
	reg.MarkActivated("active-skill")

	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithSkillRegistry(reg))

	// Turn 1: active-skill is active, available-skill is available
	_, err := loop.Run(context.Background(), []model.Message{
		{Role: model.RoleUser, Content: "turn 1"},
	}, nil)
	if err != nil {
		t.Fatalf("Run turn 1 error = %v", err)
	}

	req1 := client.requests[0]
	sys1 := req1.Messages[0].Content
	if !strings.Contains(sys1, "<active_skills>") || !strings.Contains(sys1, "Do active stuff") {
		t.Fatalf("expected active skill instructions in sys1, got: %s", sys1)
	}
	if !strings.Contains(sys1, "<available_skills>") || !strings.Contains(sys1, "available-skill") {
		t.Fatalf("expected available-skill in catalog in sys1, got: %s", sys1)
	}
	if strings.Contains(sys1, "<available_skills>") && strings.Contains(sys1, "<name>active-skill</name>") {
		t.Fatalf("active-skill should not be listed as available in sys1")
	}

	// Turn 2: deactivate active-skill -> moves to available
	reg.Deactivate("active-skill")
	_, err = loop.Run(context.Background(), []model.Message{
		{Role: model.RoleUser, Content: "turn 2"},
	}, nil)
	if err != nil {
		t.Fatalf("Run turn 2 error = %v", err)
	}

	req2 := client.requests[1]
	sys2 := req2.Messages[0].Content
	if strings.Contains(sys2, "<active_skills>") {
		t.Fatalf("expected no active_skills in sys2 after deactivation, got: %s", sys2)
	}
	if !strings.Contains(sys2, "<name>active-skill</name>") {
		t.Fatalf("expected active-skill back in available catalog in sys2, got: %s", sys2)
	}
}
