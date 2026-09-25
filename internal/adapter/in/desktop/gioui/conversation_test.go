//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func newTestController() *controller {
	client := &acpclient.Client{}
	profile := defaultACPAgentProfile()
	return &controller{
		ctx:             context.Background(),
		state:           desktopstate.State{Sessions: []desktopstate.SessionState{{ID: "session-1", AgentID: controllerAgentID, Status: desktopstate.TaskIdle}}},
		profiles:        map[string]app.ACPAgentProfile{controllerAgentID: profile},
		clients:         map[string]*acpclient.Client{controllerAgentID: client},
		connections:     map[string]connectionPhase{controllerAgentID: connectionConnected},
		statuses:        map[string]string{controllerAgentID: "Connected"},
		activeAgentID:   controllerAgentID,
		histories:       make(map[string]historyState),
		historyStaging:  make(map[string][]desktopstate.Event),
		messageStreams:  make(map[string]string),
		messageSequence: make(map[string]uint64),
		permissionWait:  make(map[string]chan string),
	}
}

func sessionChunkEvent(sessionID, kind, text string) acpclient.Event {
	payload := map[string]any{
		"sessionId": sessionID,
		"update": map[string]any{
			"sessionUpdate": kind,
			"content":       map[string]any{"type": "text", "text": text},
		},
	}
	params, _ := json.Marshal(payload)
	return acpclient.Event{Method: "session/update", Params: params}
}

func TestSessionUpdateChunksAccumulate(t *testing.T) {
	controller := newTestController()
	controller.handleACPEvent(sessionChunkEvent("session-1", "user_message_chunk", "Hel"))
	controller.handleACPEvent(sessionChunkEvent("session-1", "user_message_chunk", "lo"))
	controller.handleACPEvent(sessionChunkEvent("session-1", "agent_message_chunk", " Wor"))
	controller.handleACPEvent(sessionChunkEvent("session-1", "agent_message_chunk", "ld"))

	toolParams, _ := json.Marshal(map[string]any{
		"sessionId": "session-1",
		"update": map[string]any{
			"sessionUpdate": "tool_call",
			"toolCallId":    "tool-1",
			"title":         "Read file",
			"status":        "in_progress",
		},
	})
	controller.handleACPEvent(acpclient.Event{Method: "session/update", Params: toolParams})
	controller.handleACPEvent(sessionChunkEvent("session-1", "agent_message_chunk", "Next"))

	timeline := controller.state.Sessions[0].Timeline
	if len(timeline) != 4 {
		t.Fatalf("timeline length = %d, want 4: %#v", len(timeline), timeline)
	}
	if timeline[0].Kind != desktopstate.TimelineUser || timeline[0].Text != "Hello" {
		t.Fatalf("user timeline = %#v", timeline[0])
	}
	if timeline[0].Streaming {
		t.Fatal("user message stream remained active after the tool transition")
	}
	if timeline[1].Kind != desktopstate.TimelineAssistant || timeline[1].Text != " World" {
		t.Fatalf("assistant timeline = %#v", timeline[1])
	}
	if timeline[1].Streaming {
		t.Fatal("assistant message stream remained active after the tool transition")
	}
	if timeline[2].ID != "tool-1" || timeline[2].Text != "" {
		t.Fatalf("tool timeline = %#v", timeline[2])
	}
	if timeline[3].Kind != desktopstate.TimelineAssistant || timeline[3].Text != "Next" {
		t.Fatalf("post-tool assistant timeline = %#v", timeline[3])
	}
	if timeline[1].Streaming {
		t.Fatal("tool transition did not complete the prior assistant stream")
	}
	if !timeline[3].Streaming {
		t.Fatal("new assistant stream was not marked active")
	}
	if timeline[0].ID == timeline[1].ID || timeline[1].ID == timeline[3].ID {
		t.Fatalf("message streams reused IDs: %q, %q, %q", timeline[0].ID, timeline[1].ID, timeline[3].ID)
	}
}

func TestSessionUpdateTextAcceptsACPContentShapes(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "text block", raw: `{"type":"text","text":"hello"}`, want: "hello"},
		{name: "text block array", raw: `[{"type":"text","text":"hello"}]`, want: "hello"},
		{name: "wrapped update", raw: `[{"type":"content","content":{"type":"text","text":"hello"}}]`, want: "hello"},
		{name: "wrapped object", raw: `{"content":{"type":"text","text":"hello"}}`, want: "hello"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := sessionUpdateText(json.RawMessage(test.raw)); got != test.want {
				t.Fatalf("text = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStreamingTextBoundsLongUTF8Content(t *testing.T) {
	source := strings.Repeat("ก", maxStreamingTextBytes)
	got := streamingText(source)
	if len(got) <= maxStreamingTextBytes || !strings.HasPrefix(got, "…\n") {
		t.Fatalf("bounded stream = %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("bounded stream is not valid UTF-8: %q", got)
	}
}

func TestStaleClientEventsAreIgnored(t *testing.T) {
	controller := newTestController()
	controller.handleACPEventFor(&acpclient.Client{}, sessionChunkEvent("session-1", "agent_message_chunk", "stale"))
	if len(controller.state.Sessions[0].Timeline) != 0 {
		t.Fatalf("stale client event changed state: %#v", controller.state.Sessions[0].Timeline)
	}
}

func TestSessionUpdatesRouteThroughOwningAgent(t *testing.T) {
	controller := newTestController()
	reviewerClient := &acpclient.Client{}
	controller.clients["reviewer"] = reviewerClient
	controller.connections["reviewer"] = connectionConnected
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{ID: "review-session", AgentID: "reviewer", Status: desktopstate.TaskIdle})

	controller.handleACPEventForAgent("reviewer", reviewerClient, sessionChunkEvent("session-1", "agent_message_chunk", "wrong owner"))
	controller.handleACPEventForAgent("reviewer", reviewerClient, sessionChunkEvent("review-session", "agent_message_chunk", "owned"))

	if len(controller.state.Sessions[0].Timeline) != 0 {
		t.Fatalf("cross-agent event changed proton session: %#v", controller.state.Sessions[0].Timeline)
	}
	if len(controller.state.Sessions[1].Timeline) != 1 || controller.state.Sessions[1].Timeline[0].Text != "owned" {
		t.Fatalf("reviewer timeline = %#v", controller.state.Sessions[1].Timeline)
	}
}

func TestFinishSessionHistoryAppliesStagedEvents(t *testing.T) {
	controller := newTestController()
	client := controller.clients[controllerAgentID]
	controller.histories["session-1"] = historyStateLoading
	controller.historyStaging["session-1"] = []desktopstate.Event{{
		Kind:      desktopstate.EventTimelineAppended,
		SessionID: "session-1",
		Item:      desktopstate.TimelineItem{ID: "history-1", Kind: desktopstate.TimelineAssistant, Text: "history"},
	}}

	controller.finishSessionHistoryLoad(client, "session-1", nil)

	if got := controller.histories["session-1"]; got != historyStateLoaded {
		t.Fatalf("history state = %v, want loaded", got)
	}
	if got := controller.state.Sessions[0].Timeline; len(got) != 1 || got[0].Text != "history" {
		t.Fatalf("staged history = %#v", got)
	}
	if len(controller.historyStaging["session-1"]) != 0 {
		t.Fatalf("history staging was not cleared: %#v", controller.historyStaging["session-1"])
	}
}

func TestFinishSessionHistoryDiscardsFailedAndStaleEvents(t *testing.T) {
	for _, test := range []struct {
		name       string
		client     *acpclient.Client
		loadErr    error
		wantStatus string
	}{
		{name: "failed", client: &acpclient.Client{}, loadErr: errors.New("load failed"), wantStatus: "Session history failed"},
		{name: "stale client", client: &acpclient.Client{}, loadErr: nil, wantStatus: "Session history failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			controller := newTestController()
			current := controller.clients[controllerAgentID]
			client := current
			if test.name == "stale client" {
				client = test.client
			}
			controller.histories["session-1"] = historyStateLoading
			controller.historyStaging["session-1"] = []desktopstate.Event{{
				Kind:      desktopstate.EventTimelineAppended,
				SessionID: "session-1",
				Item:      desktopstate.TimelineItem{Kind: desktopstate.TimelineAssistant, Text: "discard me"},
			}}
			controller.finishSessionHistoryLoad(client, "session-1", test.loadErr)

			if got := controller.histories["session-1"]; got != historyStateUnloaded {
				t.Fatalf("history state = %v, want unloaded", got)
			}
			if len(controller.state.Sessions[0].Timeline) != 0 {
				t.Fatalf("discarded history was applied: %#v", controller.state.Sessions[0].Timeline)
			}
			if !strings.Contains(controller.statuses[controllerAgentID], test.wantStatus) {
				t.Fatalf("status = %q, want prefix %q", controller.statuses[controllerAgentID], test.wantStatus)
			}
		})
	}
}

func TestSendPromptDoesNotMutateBusySession(t *testing.T) {
	for _, test := range []struct {
		name   string
		status desktopstate.TaskStatus
		load   historyState
	}{
		{name: "busy", status: desktopstate.TaskRunning},
		{name: "history loading", status: desktopstate.TaskIdle, load: historyStateLoading},
	} {
		t.Run(test.name, func(t *testing.T) {
			controller := newTestController()
			controller.state.ActiveSessionID = "session-1"
			controller.state.Sessions[0].Status = test.status
			controller.histories["session-1"] = test.load

			controller.sendPrompt("do not send")

			if got := controller.state.Sessions[0].Status; got != test.status {
				t.Fatalf("status = %q, want %q", got, test.status)
			}
			if len(controller.state.Sessions[0].Timeline) != 0 {
				t.Fatalf("busy prompt changed timeline: %#v", controller.state.Sessions[0].Timeline)
			}
		})
	}
}

func TestHandlePermissionRequestReturnsSelectedOutcome(t *testing.T) {
	controller := newTestController()
	controller.state.Sessions[0].Status = desktopstate.TaskRunning
	request := acpclient.Request{
		ID:     json.RawMessage("permission-1"),
		Method: requestPermissionMethod,
		Params: json.RawMessage(`{"sessionId":"session-1","toolCall":{"title":"Run tests"},"options":[{"optionId":"allow","name":"Allow once","kind":"allow_once"},{"optionId":"reject","name":"Reject","kind":"reject_once"}]}`),
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan any, 1)
	errs := make(chan error, 1)
	go func() {
		value, err := controller.handlePermissionRequest(ctx, request)
		result <- value
		errs <- err
	}()

	deadline := time.Now().Add(time.Second)
	for {
		controller.mu.RLock()
		ready := len(controller.state.PermissionInbox) == 1
		controller.mu.RUnlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("permission request was not projected")
		}
		time.Sleep(time.Millisecond)
	}

	controller.resolvePermission("permission-1", "allow")
	select {
	case err := <-errs:
		if err != nil {
			t.Fatalf("permission handler error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("permission handler did not return")
	}
	value := <-result
	outcome, ok := value.(map[string]any)["outcome"].(map[string]any)
	if !ok || outcome["outcome"] != "selected" || outcome["optionId"] != "allow" {
		t.Fatalf("permission response = %#v", value)
	}
	if len(controller.state.PermissionInbox) != 0 {
		t.Fatalf("permission inbox was not cleared: %#v", controller.state.PermissionInbox)
	}
	if got := controller.state.Sessions[0].Status; got != desktopstate.TaskRunning {
		t.Fatalf("session status = %q, want running", got)
	}
}

func TestPermissionDetailIsBounded(t *testing.T) {
	detail := permissionDetail(map[string]any{"input": strings.Repeat("x", maxPermissionDetailBytes*2)})
	if len(detail) > maxPermissionDetailBytes {
		t.Fatalf("permission detail length = %d, unexpectedly large", len(detail))
	}
	if !strings.Contains(detail, "truncated") {
		t.Fatalf("permission detail does not indicate truncation: %q", detail[len(detail)-32:])
	}
	detail = permissionDetail(map[string]any{"input": strings.Repeat("ก", maxPermissionDetailBytes)})
	if !utf8.ValidString(detail) {
		t.Fatalf("permission detail is not valid UTF-8: %q", detail)
	}
}
