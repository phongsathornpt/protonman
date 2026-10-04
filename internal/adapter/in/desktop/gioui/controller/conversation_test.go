//go:build desktop || desktop_gio

package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func newTestController() *controller {
	client := &acpclient.Client{}
	profile := DefaultACPAgentProfile()
	return &controller{
		ctx:                 context.Background(),
		state:               desktopstate.State{ActiveSessionID: "session-1", Sessions: []desktopstate.SessionState{{ID: "session-1", AgentID: ProtonmanAgentID, Status: desktopstate.TaskIdle}}},
		profiles:            map[string]app.ACPAgentProfile{ProtonmanAgentID: profile},
		clients:             map[string]*acpclient.Client{ProtonmanAgentID: client},
		connections:         map[string]ConnectionPhase{ProtonmanAgentID: ConnectionConnected},
		statuses:            map[string]string{ProtonmanAgentID: "Connected"},
		activeAgentID:       ProtonmanAgentID,
		histories:           make(map[string]HistoryState),
		historyStaging:      make(map[string][]desktopstate.Event),
		historyStagingBytes: make(map[string]int), historyStagingTruncated: make(map[string]bool),
		timelineBytes:        make(map[string]int),
		messageStreams:       make(map[messageStreamKey]string),
		messageStreamBuffers: make(map[messageStreamKey]*messageStreamBuffer),
		messageSequence:      make(map[string]uint64),
		permissionWait:       make(map[string]chan string),
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
	controller.state.ActiveSessionID = "session-1"
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
	controller.mu.Lock()
	controller.flushMessageStreamsLocked("session-1")
	controller.mu.Unlock()

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

func TestAssistantChunksFlushWithoutWaitingForNextProtocolEvent(t *testing.T) {
	controller := newTestController()
	notified := make(chan struct{}, 1)
	controller.onChange = func() {
		select {
		case notified <- struct{}{}:
		default:
		}
	}
	controller.handleACPEvent(sessionChunkEvent("session-1", "agent_message_chunk", "streamed text"))

	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Fatal("streaming text was not flushed to the desktop")
	}
	controller.mu.RLock()
	got := controller.state.Sessions[0].Timeline
	controller.mu.RUnlock()
	if len(got) != 1 || got[0].Text != "streamed text" || !got[0].Streaming {
		t.Fatalf("streaming timeline = %#v", got)
	}
	controller.mu.Lock()
	controller.clearMessageStreamsLocked("session-1")
	controller.mu.Unlock()
}

func TestStreamFlushAdvancesSnapshotCacheAndKeepsPreviousSnapshotImmutable(t *testing.T) {
	controller := newTestController()
	controller.state.Sessions[0].Timeline = []desktopstate.TimelineItem{{
		ID: "assistant-1", Kind: desktopstate.TimelineAssistant, Text: "before",
	}}
	controller.revision = 1
	prior := controller.Snapshot()

	key := messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}
	buffer := &messageStreamBuffer{sessionID: key.sessionID, itemID: "assistant-1", kind: desktopstate.TimelineAssistant}
	buffer.text.WriteString("after")
	controller.messageStreamBuffers[key] = buffer
	controller.flushMessageStream(key, buffer)

	current := controller.Snapshot()
	if got := current.State.Sessions[0].Timeline[0].Text; got != "after" {
		t.Fatalf("flushed snapshot text = %q, want after", got)
	}
	if got := prior.State.Sessions[0].Timeline[0].Text; got != "before" {
		t.Fatalf("stream flush mutated a previously returned snapshot: %q", got)
	}
	if current.Revision != prior.Revision+1 {
		t.Fatalf("stream snapshot revision = %d, prior revision = %d", current.Revision, prior.Revision)
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
		{name: "whitespace padded array", raw: " \n [{\"type\":\"text\",\"text\":\"hello\"}] \t", want: "hello"},
		{name: "null", raw: "null", want: ""},
		{name: "unsupported scalar", raw: "true", want: ""},
		{name: "malformed object", raw: "{", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := sessionUpdateText(json.RawMessage(test.raw)); got != test.want {
				t.Fatalf("text = %q, want %q", got, test.want)
			}
		})
	}
}

func TestStaleClientEventsAreIgnored(t *testing.T) {
	controller := newTestController()
	controller.handleACPEventFor(&acpclient.Client{}, sessionChunkEvent("session-1", "agent_message_chunk", "stale"))
	if len(controller.state.Sessions[0].Timeline) != 0 {
		t.Fatalf("stale client event changed state: %#v", controller.state.Sessions[0].Timeline)
	}
}

func TestUnknownSessionEventsDoNotAllocateTransientState(t *testing.T) {
	controller := newTestController()
	controller.handleACPEvent(sessionChunkEvent("removed-session", "agent_message_chunk", "orphaned"))

	if len(controller.messageStreams) != 0 || len(controller.messageSequence) != 0 {
		t.Fatalf("unknown session retained stream state: streams=%v sequence=%v", controller.messageStreams, controller.messageSequence)
	}
	if len(controller.state.Sessions[0].Timeline) != 0 {
		t.Fatalf("unknown session changed visible timeline: %#v", controller.state.Sessions[0].Timeline)
	}
}

func TestUnknownSessionPermissionRequestsAreRejected(t *testing.T) {
	controller := newTestController()
	request := acpclient.Request{
		ID:     json.RawMessage(`"permission-1"`),
		Method: requestPermissionMethod,
		Params: json.RawMessage(`{"sessionId":"removed-session","options":[{"optionId":"allow","name":"Allow"}]}`),
	}
	if _, err := controller.handlePermissionRequest(context.Background(), request); err == nil {
		t.Fatal("permission request for an unknown session was accepted")
	}
	if len(controller.permissionWait) != 0 || len(controller.state.PermissionInbox) != 0 {
		t.Fatalf("unknown permission request retained state: waiters=%v inbox=%v", controller.permissionWait, controller.state.PermissionInbox)
	}
}

func TestSelectSessionRetainsImmediatelyPreviousLoadedHistory(t *testing.T) {
	controller := newTestController()
	controller.clients = nil
	controller.state.ActiveSessionID = "session-1"
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{
		ID: "session-2", AgentID: ProtonmanAgentID,
		Timeline:  []desktopstate.TimelineItem{{ID: "old", Kind: desktopstate.TimelineAssistant, Text: "cached history"}},
		Subagents: []desktopstate.SubagentState{{ID: "child", Summary: "cached result"}},
		Context:   desktopstate.SessionContextState{Goal: "cached goal", Memory: desktopstate.MemoryState{Workspace: []desktopstate.MemoryEntryState{{Value: "cached memory"}}}},
		Runtime:   desktopstate.RuntimeSettingsState{Model: "cached model"},
	})
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{
		ID: "session-3", AgentID: ProtonmanAgentID,
		Timeline: []desktopstate.TimelineItem{{ID: "older", Kind: desktopstate.TimelineAssistant, Text: "older history"}},
	})
	controller.state.Sessions[0].Timeline = []desktopstate.TimelineItem{{ID: "current", Kind: desktopstate.TimelineAssistant, Text: "active history"}}
	controller.state.Sessions[0].Subagents = []desktopstate.SubagentState{{ID: "previous-child", Summary: "previous result"}}
	controller.state.Sessions[0].Context = desktopstate.SessionContextState{
		Goal:   "previous goal",
		Memory: desktopstate.MemoryState{Workspace: []desktopstate.MemoryEntryState{{Value: "previous memory"}}},
	}
	controller.state.Sessions[0].Runtime = desktopstate.RuntimeSettingsState{Model: "previous model"}
	controller.histories["session-1"] = HistoryStateLoaded
	controller.histories["session-2"] = HistoryStateLoaded
	controller.histories["session-3"] = HistoryStateLoaded
	controller.messageStreams[messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}] = "stream"

	controller.selectSession("session-2")

	if len(controller.state.Sessions[0].Timeline) != 1 {
		t.Fatalf("previous transcript retained %d items, want 1", len(controller.state.Sessions[0].Timeline))
	}
	if len(controller.state.Sessions[0].Subagents) != 1 {
		t.Fatalf("previous subagent history retained %d items, want 1", len(controller.state.Sessions[0].Subagents))
	}
	if controller.state.Sessions[0].Context.Goal != "previous goal" || len(controller.state.Sessions[0].Context.Memory.Workspace) != 1 || controller.state.Sessions[0].Runtime.Model != "previous model" {
		t.Fatal("previous session inspector state was not retained")
	}
	if controller.histories["session-1"] != HistoryStateLoaded {
		t.Fatalf("previous history state = %v, want loaded", controller.histories["session-1"])
	}
	if controller.state.Sessions[1].Timeline[0].Text != "cached history" {
		t.Fatal("selected session history was evicted")
	}
	if len(controller.state.Sessions[2].Timeline) != 0 || controller.histories["session-3"] != HistoryStateUnloaded {
		t.Fatal("older inactive history was not evicted")
	}
	if _, ok := controller.messageStreams[messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}]; ok {
		t.Fatal("inactive stream retained")
	}

	controller.selectSession("session-1")
	if got := controller.state.Sessions[0].Timeline[0].Text; got != "active history" {
		t.Fatalf("reselected session history = %q, want retained history", got)
	}
	if controller.histories["session-1"] != HistoryStateLoaded {
		t.Fatalf("reselected history state = %v, want loaded", controller.histories["session-1"])
	}
}

func TestSelectSessionEvictsPreviousHistoryWhileItCanReceiveUpdates(t *testing.T) {
	controller := newTestController()
	controller.clients = nil
	controller.state.ActiveSessionID = "session-1"
	controller.state.Sessions[0].Status = desktopstate.TaskRunning
	controller.state.Sessions[0].Timeline = []desktopstate.TimelineItem{{ID: "current", Kind: desktopstate.TimelineAssistant, Text: "active history"}}
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{ID: "session-2", AgentID: ProtonmanAgentID})
	controller.histories["session-1"] = HistoryStateLoaded
	controller.histories["session-2"] = HistoryStateLoaded

	controller.selectSession("session-2")

	if len(controller.state.Sessions[0].Timeline) != 0 || controller.histories["session-1"] != HistoryStateUnloaded {
		t.Fatal("inactive running session history was retained despite dropped updates")
	}
}

func TestApplyStagedHistoryEventsMatchesIncrementalApplication(t *testing.T) {
	events := []desktopstate.Event{
		{
			Kind: desktopstate.EventTimelineAppended, SessionID: "session-a",
			Item: desktopstate.TimelineItem{ID: "assistant-1", Kind: desktopstate.TimelineAssistant, Text: "hello", Streaming: true},
		},
		{
			Kind: desktopstate.EventTimelineUpserted, SessionID: "session-a",
			Item: desktopstate.TimelineItem{ID: "assistant-1", Kind: desktopstate.TimelineAssistant, Text: " world"},
		},
		{
			Kind: desktopstate.EventTimelineAppended, SessionID: "session-a",
			Item: desktopstate.TimelineItem{ID: "tool-1", Kind: desktopstate.TimelineTool, Title: "read", Status: "completed"},
		},
		{
			Kind: desktopstate.EventSubagentUpserted, SessionID: "session-a",
			Subagent: desktopstate.SubagentState{ID: "child-1", Profile: "strength", Summary: "done"},
		},
	}
	incremental := benchmarkController()
	batched := benchmarkController()
	incremental.state.Sessions[0].Timeline = nil
	batched.state.Sessions[0].Timeline = nil

	incremental.mu.Lock()
	for _, event := range events {
		incremental.applyTimelineEventLocked(event)
	}
	incremental.mu.Unlock()
	batched.mu.Lock()
	batched.applyStagedHistoryEventsLocked("session-a", events)
	batched.mu.Unlock()

	if !reflect.DeepEqual(incremental.state.Sessions[0], batched.state.Sessions[0]) {
		t.Fatalf("batched session state differs from incremental application:\n got: %#v\nwant: %#v", batched.state.Sessions[0], incremental.state.Sessions[0])
	}
	if !reflect.DeepEqual(incremental.timelineBytes, batched.timelineBytes) {
		t.Fatalf("batched timeline accounting = %#v, want %#v", batched.timelineBytes, incremental.timelineBytes)
	}
}

func TestInactiveSessionUpdatesDoNotRetainTimeline(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "different-session"
	controller.handleACPEvent(sessionChunkEvent("session-1", "agent_message_chunk", strings.Repeat("old transcript ", 100)))

	if len(controller.state.Sessions[0].Timeline) != 0 {
		t.Fatalf("inactive session retained timeline update: %#v", controller.state.Sessions[0].Timeline)
	}
	if len(controller.messageStreams) != 0 {
		t.Fatalf("inactive session retained stream state: %#v", controller.messageStreams)
	}
}

func TestInactiveLoadingSessionDoesNotRetainStreamUpdates(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{
		ID: "session-2", AgentID: ProtonmanAgentID,
	})
	controller.histories["session-2"] = HistoryStateLoading

	controller.handleACPEvent(sessionChunkEvent("session-2", "agent_message_chunk", strings.Repeat("inactive chunk ", 256)))

	if len(controller.historyStaging["session-2"]) != 0 || controller.historyStagingBytes["session-2"] != 0 {
		t.Fatalf("inactive history load retained staged updates: events=%d bytes=%d", len(controller.historyStaging["session-2"]), controller.historyStagingBytes["session-2"])
	}
	if len(controller.messageStreams) != 0 || len(controller.messageStreamBuffers) != 0 {
		t.Fatalf("inactive history load retained stream state: streams=%v buffers=%v", controller.messageStreams, controller.messageStreamBuffers)
	}
}

func TestSelectingAnotherSessionReleasesInactiveHistoryStaging(t *testing.T) {
	controller := newTestController()
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{
		ID: "session-2", AgentID: ProtonmanAgentID,
	})
	controller.histories["session-2"] = HistoryStateLoading
	controller.historyLoads = make(map[string]*sessionHistoryLoad)
	loadCtx, cancelLoad := context.WithCancel(context.Background())
	controller.historyLoads["session-2"] = &sessionHistoryLoad{cancel: cancelLoad}
	controller.historyStaging["session-2"] = []desktopstate.Event{{
		Kind: desktopstate.EventTimelineAppended, SessionID: "session-2",
		Item: desktopstate.TimelineItem{Text: strings.Repeat("x", 1024)},
	}}
	controller.historyStagingBytes["session-2"] = 1200
	controller.historyStagingTruncated["session-2"] = true

	controller.mu.Lock()
	controller.pruneInactiveSessionHistoryLocked("session-1", "")
	controller.mu.Unlock()

	if _, ok := controller.historyStaging["session-2"]; ok {
		t.Fatal("inactive session staging events were retained")
	}
	if _, ok := controller.historyStagingBytes["session-2"]; ok {
		t.Fatal("inactive session staging byte count was retained")
	}
	if _, ok := controller.historyStagingTruncated["session-2"]; ok {
		t.Fatal("inactive session truncation flag was retained")
	}
	if controller.histories["session-2"] != HistoryStateUnloaded {
		t.Fatalf("cancelled history state = %v, want unloaded", controller.histories["session-2"])
	}
	if _, ok := controller.historyLoads["session-2"]; ok {
		t.Fatal("inactive history request was retained")
	}
	select {
	case <-loadCtx.Done():
	default:
		t.Fatal("inactive history request context was not cancelled")
	}
}

func TestPruneSessionRuntimeRemovesEveryTransientEntryForDeletedSessions(t *testing.T) {
	controller := newTestController()
	controller.state.PermissionInbox = []desktopstate.PermissionRequest{{RequestID: "live-permission", SessionID: "session-1"}}
	controller.histories["session-1"] = HistoryStateLoaded
	controller.histories["removed"] = HistoryStateLoaded
	controller.timelineBytes["removed"] = 128
	controller.historyLoads = make(map[string]*sessionHistoryLoad)
	removedLoadCtx, cancelRemovedLoad := context.WithCancel(context.Background())
	controller.historyLoads["removed"] = &sessionHistoryLoad{cancel: cancelRemovedLoad}
	controller.historyStaging["removed"] = []desktopstate.Event{{Kind: desktopstate.EventTimelineAppended}}
	controller.historyStagingBytes["removed"] = 128
	controller.messageSequence["removed"] = 4
	controller.messageSequence["session-1"] = 2
	controller.messageStreams[messageStreamKey{sessionID: "removed", kind: "agent_message_chunk"}] = "old-stream"
	controller.messageStreams[messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}] = "live-stream"
	controller.messageStreams[messageStreamKey{agentID: "reviewer", sessionID: "session-1", kind: "agent_message_chunk"}] = "orphaned-colliding-stream"
	controller.messageStreamBuffers[messageStreamKey{agentID: "reviewer", sessionID: "session-1", kind: "agent_message_chunk"}] = &messageStreamBuffer{}
	controller.permissionWait["live-permission"] = make(chan string, 1)
	removedWaiter := make(chan string, 1)
	controller.permissionWait["removed-permission"] = removedWaiter
	questionKey := SessionRefStorageKey(desktopstate.SessionRef{AgentID: "reviewer", SessionID: "removed-question"})
	controller.state.QuestionInbox = []desktopstate.QuestionRequest{{RequestID: questionKey, AgentID: "reviewer", SessionID: "session-1"}}
	questionWaiter := make(chan desktopstate.QuestionResponse, 1)
	if controller.questionWait == nil {
		controller.questionWait = make(map[string]chan desktopstate.QuestionResponse)
	}
	controller.questionWait[questionKey] = questionWaiter
	controller.runtimeMutation = "removed"

	controller.mu.Lock()
	controller.pruneSessionRuntimeLocked()
	controller.mu.Unlock()

	if _, ok := controller.histories["removed"]; ok {
		t.Fatal("deleted session history state was retained")
	}
	if _, ok := controller.historyStaging["removed"]; ok {
		t.Fatal("deleted session staged events were retained")
	}
	if _, ok := controller.historyStagingBytes["removed"]; ok {
		t.Fatal("deleted session staging byte count was retained")
	}
	if _, ok := controller.timelineBytes["removed"]; ok {
		t.Fatal("deleted session timeline byte count was retained")
	}
	if _, ok := controller.historyLoads["removed"]; ok {
		t.Fatal("deleted session history load was retained")
	}
	select {
	case <-removedLoadCtx.Done():
	default:
		t.Fatal("deleted session history load was not cancelled")
	}
	if _, ok := controller.messageSequence["removed"]; ok {
		t.Fatal("deleted session sequence was retained")
	}
	if _, ok := controller.messageStreams[messageStreamKey{sessionID: "removed", kind: "agent_message_chunk"}]; ok {
		t.Fatal("deleted session stream was retained")
	}
	if _, ok := controller.messageStreams[messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}]; !ok {
		t.Fatal("live session stream was pruned")
	}
	if _, ok := controller.messageStreams[messageStreamKey{agentID: "reviewer", sessionID: "session-1", kind: "agent_message_chunk"}]; ok {
		t.Fatal("same-ID stream for a removed agent was retained")
	}
	if _, ok := controller.permissionWait["removed-permission"]; ok {
		t.Fatal("orphaned permission waiter was retained")
	}
	if outcome := <-removedWaiter; outcome != "" {
		t.Fatalf("orphaned permission outcome = %q, want cancellation", outcome)
	}
	if controller.permissionWait["live-permission"] == nil {
		t.Fatal("live permission waiter was pruned")
	}
	if len(controller.state.QuestionInbox) != 0 || controller.questionWait[questionKey] != nil {
		t.Fatal("question for a removed agent/session was retained")
	}
	if response := <-questionWaiter; response.Status != "declined" || response.Answer != "session ended" {
		t.Fatalf("orphaned question response = %+v", response)
	}
	if controller.runtimeMutation != "" {
		t.Fatalf("deleted session runtime mutation = %q", controller.runtimeMutation)
	}
}

func TestSessionUpdatesRouteThroughOwningAgent(t *testing.T) {
	controller := newTestController()
	reviewerClient := &acpclient.Client{}
	controller.clients["reviewer"] = reviewerClient
	controller.connections["reviewer"] = ConnectionConnected
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{ID: "review-session", AgentID: "reviewer", Status: desktopstate.TaskIdle})
	controller.state.ActiveSessionID = "review-session"

	controller.handleACPEventForAgent("reviewer", reviewerClient, sessionChunkEvent("session-1", "agent_message_chunk", "wrong owner"))
	controller.handleACPEventForAgent("reviewer", reviewerClient, sessionChunkEvent("review-session", "agent_message_chunk", "owned"))
	controller.mu.Lock()
	controller.flushMessageStreamsLocked("review-session")
	controller.mu.Unlock()

	if len(controller.state.Sessions[0].Timeline) != 0 {
		t.Fatalf("cross-agent event changed proton session: %#v", controller.state.Sessions[0].Timeline)
	}
	if len(controller.state.Sessions[1].Timeline) != 1 || controller.state.Sessions[1].Timeline[0].Text != "owned" {
		t.Fatalf("reviewer timeline = %#v", controller.state.Sessions[1].Timeline)
	}
}

func TestPruneSessionRuntimeCleansQuestionsWithoutPermissionWaiters(t *testing.T) {
	controller := newTestController()
	requestID := SessionRefStorageKey(desktopstate.SessionRef{AgentID: "reviewer", SessionID: "removed"})
	waiter := make(chan desktopstate.QuestionResponse, 1)
	controller.state.QuestionInbox = []desktopstate.QuestionRequest{{RequestID: requestID, AgentID: "reviewer", SessionID: "removed"}}
	controller.questionWait = map[string]chan desktopstate.QuestionResponse{requestID: waiter}

	controller.mu.Lock()
	controller.pruneSessionRuntimeLocked()
	controller.mu.Unlock()

	if len(controller.state.QuestionInbox) != 0 {
		t.Fatalf("orphaned question inbox = %#v, want empty", controller.state.QuestionInbox)
	}
	if _, ok := controller.questionWait[requestID]; ok {
		t.Fatal("orphaned question waiter was retained without permission waiters")
	}
	select {
	case response := <-waiter:
		if response.Status != "declined" || response.Answer != "session ended" {
			t.Fatalf("orphaned question response = %+v, want session-ended decline", response)
		}
	default:
		t.Fatal("orphaned question waiter was not released")
	}
}

func TestFinishSessionHistoryAppliesStagedEvents(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	controller.contextRefresh = &sessionRefreshTracker{inFlight: map[string]bool{"session-1": true}}
	controller.memoryRefresh = &sessionRefreshTracker{inFlight: map[string]bool{"session-1": true}}
	controller.runtimeRefresh = &sessionRefreshTracker{inFlight: map[string]bool{"session-1": true}}
	client := controller.clients[ProtonmanAgentID]
	controller.histories["session-1"] = HistoryStateLoading
	controller.historyStaging["session-1"] = []desktopstate.Event{{
		Kind:      desktopstate.EventTimelineAppended,
		SessionID: "session-1",
		Item:      desktopstate.TimelineItem{ID: "history-1", Kind: desktopstate.TimelineAssistant, Text: "history "},
	}, {
		Kind:      desktopstate.EventTimelineUpserted,
		SessionID: "session-1",
		Item:      desktopstate.TimelineItem{ID: "history-1", Kind: desktopstate.TimelineAssistant, Text: "loaded", Streaming: true},
	}}

	controller.finishSessionHistoryLoad(client, "session-1", nil)

	if got := controller.histories["session-1"]; got != HistoryStateLoaded {
		t.Fatalf("history state = %v, want loaded", got)
	}
	if got := controller.state.Sessions[0].Timeline; len(got) != 1 || got[0].Text != "history loaded" || got[0].Streaming {
		t.Fatalf("staged history = %#v", got)
	}
	if len(controller.historyStaging["session-1"]) != 0 {
		t.Fatalf("history staging was not cleared: %#v", controller.historyStaging["session-1"])
	}
}

func TestFinishSessionHistoryTrimsOverflowedStagingToRecentEvents(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	controller.contextRefresh = &sessionRefreshTracker{inFlight: map[string]bool{"session-1": true}}
	controller.memoryRefresh = &sessionRefreshTracker{inFlight: map[string]bool{"session-1": true}}
	controller.runtimeRefresh = &sessionRefreshTracker{inFlight: map[string]bool{"session-1": true}}
	controller.histories["session-1"] = HistoryStateLoading
	controller.stageHistoryEventLocked("session-1", desktopstate.Event{
		Kind: desktopstate.EventTimelineAppended, SessionID: "session-1",
		Item: desktopstate.TimelineItem{Kind: desktopstate.TimelineAssistant, Text: strings.Repeat("x", maxHistoryStagingBytes)},
	})
	controller.stageHistoryEventLocked("session-1", desktopstate.Event{
		Kind: desktopstate.EventTimelineAppended, SessionID: "session-1",
		Item: desktopstate.TimelineItem{Kind: desktopstate.TimelineAssistant, Text: "kept recent"},
	})

	controller.finishSessionHistoryLoad(controller.clients[ProtonmanAgentID], "session-1", nil)

	if controller.histories["session-1"] != HistoryStateLoaded {
		t.Fatalf("history state = %v, want loaded", controller.histories["session-1"])
	}
	if got := controller.state.Sessions[0].Timeline; len(got) != 1 || got[0].Text != "kept recent" {
		t.Fatalf("recent staged history = %#v", got)
	}
	if !controller.state.Sessions[0].HistoryTruncated {
		t.Fatal("truncated history did not set the presentation notice")
	}
	if controller.historyStagingBytes["session-1"] != 0 || len(controller.historyStaging["session-1"]) != 0 {
		t.Fatal("history staging retained data after completion")
	}
}

func TestTrustedPruneKeepsByteAccountingAccurate(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	text := strings.Repeat("y", 1024)
	for index := 0; index < maxSessionTimelineItems+64; index++ {
		controller.applyTimelineEventLocked(desktopstate.Event{
			Kind:      desktopstate.EventTimelineAppended,
			SessionID: "session-1",
			Item: desktopstate.TimelineItem{
				ID:   fmt.Sprintf("item-%d", index),
				Kind: desktopstate.TimelineAssistant,
				Text: text,
			},
		})
	}

	session := controller.state.Sessions[0]
	if len(session.Timeline) > maxSessionTimelineItems {
		t.Fatalf("retained items = %d, limit %d", len(session.Timeline), maxSessionTimelineItems)
	}
	if got, want := controller.timelineBytes["session-1"], retainedTimelineBytes(session.Timeline); got != want {
		t.Fatalf("trusted byte accounting = %d, actual retained = %d", got, want)
	}
	if !session.HistoryTruncated {
		t.Fatal("trusted prune did not flag truncated history")
	}
}

func TestActiveTimelineRetentionIsBounded(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	items := make([]desktopstate.TimelineItem, maxSessionTimelineItems)
	for index := range items {
		items[index] = desktopstate.TimelineItem{
			ID: fmt.Sprintf("item-%d", index), Kind: desktopstate.TimelineAssistant,
			Text: strings.Repeat("x", 1024),
		}
	}
	controller.state.Sessions[0].Timeline = items
	controller.timelineBytes["session-1"] = timelineSize(items)

	controller.applyTimelineEventLocked(desktopstate.Event{
		Kind: desktopstate.EventTimelineAppended, SessionID: "session-1",
		Item: desktopstate.TimelineItem{ID: "newest", Kind: desktopstate.TimelineAssistant, Text: "latest"},
	})

	retained := controller.state.Sessions[0].Timeline
	if len(retained) > maxSessionTimelineItems || controller.timelineBytes["session-1"] > maxSessionTimelineBytes {
		t.Fatalf("timeline bounds exceeded: items=%d bytes=%d", len(retained), controller.timelineBytes["session-1"])
	}
	if len(retained) == 0 || retained[len(retained)-1].ID != "newest" {
		t.Fatal("newest timeline item was not retained")
	}
	if !controller.state.Sessions[0].HistoryTruncated {
		t.Fatal("timeline trimming did not set the presentation notice")
	}
}

func TestDesktopHistoryRetentionSoak(t *testing.T) {
	var previousRetained uint64
	for cycle := range 3 {
		retained := desktopHistoryRetentionSoakCycle(t)
		if cycle > 0 {
			delta := int64(retained) - int64(previousRetained)
			if delta < 0 {
				delta = -delta
			}
			if delta > 8<<20 {
				t.Fatalf("retained heap changed by %d bytes between soak cycles", delta)
			}
		}
		previousRetained = retained
	}
}

func desktopHistoryRetentionSoakCycle(t *testing.T) uint64 {
	t.Helper()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	controller := newTestController()
	textTail := strings.Repeat("x", 504)
	for index := 0; index < 120_000; index++ {
		text := fmt.Sprintf("%08d%s", index, textTail)
		controller.mu.Lock()
		event, ok := controller.eventsForSessionUpdateLocked(sessionUpdatePayload{SessionID: "session-1"}, desktopstate.SessionUpdate{
			SessionID: "session-1", Kind: "agent_message_chunk", Text: text,
		})
		if !ok {
			controller.mu.Unlock()
			t.Fatal("assistant stream event was rejected")
		}
		controller.appendMessageChunkLocked("session-1", "agent_message_chunk", event)
		controller.stageHistoryEventLocked("session-1", desktopstate.Event{
			Kind: desktopstate.EventTimelineAppended, SessionID: "session-1",
			Item: desktopstate.TimelineItem{ID: fmt.Sprintf("history-%d", index), Kind: desktopstate.TimelineAssistant, Text: strings.Clone(text)},
		})
		controller.mu.Unlock()
	}
	controller.mu.Lock()
	controller.clearMessageStreamsLocked("session-1")
	controller.mu.Unlock()

	if got := len(controller.state.Sessions[0].Timeline); got > maxSessionTimelineItems {
		t.Fatalf("soak timeline items = %d, limit %d", got, maxSessionTimelineItems)
	}
	if got := controller.timelineBytes["session-1"]; got > maxSessionTimelineBytes {
		t.Fatalf("soak timeline bytes = %d, limit %d", got, maxSessionTimelineBytes)
	}
	if len(controller.state.Sessions[0].Timeline) == 0 || len(controller.state.Sessions[0].Timeline[0].Text) > maxMessageStreamBytes {
		got := 0
		if len(controller.state.Sessions[0].Timeline) > 0 {
			got = len(controller.state.Sessions[0].Timeline[0].Text)
		}
		t.Fatalf("soak active stream bytes = %d, limit %d", got, maxMessageStreamBytes)
	}
	if len(controller.messageStreams) != 0 || len(controller.messageStreamBuffers) != 0 {
		t.Fatal("completed stream retained timer or buffer state")
	}
	if got := len(controller.historyStaging["session-1"]); got > maxHistoryStagingEvents {
		t.Fatalf("soak staged events = %d, limit %d", got, maxHistoryStagingEvents)
	}
	if got := controller.historyStagingBytes["session-1"]; got > maxHistoryStagingBytes {
		t.Fatalf("soak staged bytes = %d, limit %d", got, maxHistoryStagingBytes)
	}

	runtime.GC()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(controller)
	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	t.Logf("heap alloc before=%d after=%d delta=%d bytes", before.HeapAlloc, after.HeapAlloc, growth)
	if growth > 24<<20 {
		t.Fatalf("retained heap grew by %d bytes after capped history soak", growth)
	}
	return after.HeapAlloc
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
			current := controller.clients[ProtonmanAgentID]
			client := current
			if test.name == "stale client" {
				client = test.client
			}
			controller.histories["session-1"] = HistoryStateLoading
			controller.historyStaging["session-1"] = []desktopstate.Event{{
				Kind:      desktopstate.EventTimelineAppended,
				SessionID: "session-1",
				Item:      desktopstate.TimelineItem{Kind: desktopstate.TimelineAssistant, Text: "discard me"},
			}}
			controller.finishSessionHistoryLoad(client, "session-1", test.loadErr)

			if got := controller.histories["session-1"]; got != HistoryStateUnloaded {
				t.Fatalf("history state = %v, want unloaded", got)
			}
			if len(controller.state.Sessions[0].Timeline) != 0 {
				t.Fatalf("discarded history was applied: %#v", controller.state.Sessions[0].Timeline)
			}
			if !strings.Contains(controller.statuses[ProtonmanAgentID], test.wantStatus) {
				t.Fatalf("status = %q, want prefix %q", controller.statuses[ProtonmanAgentID], test.wantStatus)
			}
		})
	}
}

func TestSendPromptDoesNotMutateBusySession(t *testing.T) {
	for _, test := range []struct {
		name   string
		status desktopstate.TaskStatus
		load   HistoryState
	}{
		{name: "busy", status: desktopstate.TaskRunning},
		{name: "history loading", status: desktopstate.TaskIdle, load: HistoryStateLoading},
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
	controller.state.Sessions[0].Title = "Cline ACP"
	controller.state.Sessions[0].Status = desktopstate.TaskRunning
	request := acpclient.Request{
		ID:     json.RawMessage("7"),
		Method: requestPermissionMethod,
		Params: json.RawMessage(`{"sessionId":"session-1","toolCall":{"toolCallId":"tool-1","title":"Run command: go test ./...","kind":"execute","status":"pending","rawInput":{"command":"go test ./..."}},"options":[{"optionId":"allow_once","name":"Allow once","kind":"allow_once"},{"optionId":"allow_always","name":"Allow always","kind":"allow_always"},{"optionId":"reject_once","name":"Reject","kind":"reject_once"}]}`),
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
			controller.mu.RLock()
			permission := controller.state.PermissionInbox[0]
			status := controller.statuses[ProtonmanAgentID]
			controller.mu.RUnlock()
			if permission.Title != "Run command: go test ./..." || permission.Command != "go test ./..." {
				t.Fatalf("Cline permission details = title %q command %q", permission.Title, permission.Command)
			}
			if len(permission.Options) != 3 || permission.Options[0].ID != "allow_once" || permission.Options[1].ID != "allow_always" || permission.Options[2].ID != "reject_once" {
				t.Fatalf("Cline permission options = %#v", permission.Options)
			}
			if status != "Permission required · Cline ACP" {
				t.Fatalf("Cline permission status = %q", status)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("permission request was not projected")
		}
		time.Sleep(time.Millisecond)
	}

	controller.ResolvePermission("7", "allow_once")
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
	if !ok || outcome["outcome"] != "selected" || outcome["optionId"] != "allow_once" {
		t.Fatalf("permission response = %#v", value)
	}
	if len(controller.state.PermissionInbox) != 0 {
		t.Fatalf("permission inbox was not cleared: %#v", controller.state.PermissionInbox)
	}
	if got := controller.state.Sessions[0].Status; got != desktopstate.TaskRunning {
		t.Fatalf("session status = %q, want running", got)
	}
	if len(controller.state.Sessions[0].Timeline) == 0 {
		t.Fatal("expected audit timeline item after permission resolved, got none")
	}
	if controller.state.Sessions[0].Timeline[0].Kind != desktopstate.TimelinePermission {
		t.Fatalf("expected TimelinePermission, got %v", controller.state.Sessions[0].Timeline[0].Kind)
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

func TestLogSessionLoadTimingReportsServerStages(t *testing.T) {
	t.Setenv(envconfig.Timing, "1")
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	meta := json.RawMessage(`{"protonman":{"timings":{"totalUs":12345,"replayUs":4000,"replayMessages":256,"diskLoadUs":900}}}`)
	request := &sessionHistoryLoad{startedAt: time.Now().Add(-20 * time.Millisecond), serverMeta: meta}
	logSessionLoadTiming("session-1", request, nil)

	out := buf.String()
	if !strings.Contains(out, "[TIMING] session/load") {
		t.Fatalf("timing log missing header: %q", out)
	}
	if !strings.Contains(out, "(256 messages)") {
		t.Fatalf("timing log missing replay message count: %q", out)
	}
	if !strings.Contains(out, "diskLoad=900µs") && !strings.Contains(out, "diskLoad=1ms") {
		t.Fatalf("timing log missing disk load stage: %q", out)
	}
}

func TestLogSessionLoadTimingSilentWhenDisabled(t *testing.T) {
	t.Setenv(envconfig.Timing, "")
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	logSessionLoadTiming("session-1", &sessionHistoryLoad{startedAt: time.Now()}, nil)
	logSessionLoadTiming("session-1", nil, errors.New("boom"))

	if strings.Contains(buf.String(), "[TIMING]") {
		t.Fatalf("timing log emitted while disabled: %q", buf.String())
	}
}

func TestSessionHistoryLoadParamsDeferModelDiscovery(t *testing.T) {
	controller := newTestController()
	params := controller.sessionHistoryLoadParams("session-1", "/workspace", []string{"/workspace/extra"})
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Meta struct {
			Protonman struct {
				DeferModelDiscovery bool `json:"deferModelDiscovery"`
			} `json:"protonman"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Meta.Protonman.DeferModelDiscovery {
		t.Fatalf("session history load metadata = %s, want deferred model discovery", encoded)
	}
	if params["sessionId"] != "session-1" || params["cwd"] != "/workspace" {
		t.Fatalf("session history load params lost session/workspace: %#v", params)
	}
}

func TestTimelineItemIndexRepairsStaleCache(t *testing.T) {
	session := &desktopstate.SessionState{Timeline: []desktopstate.TimelineItem{
		{ID: "a", Kind: desktopstate.TimelineAssistant},
		{ID: "b", Kind: desktopstate.TimelineTool},
		{ID: "c", Kind: desktopstate.TimelineAssistant},
	}}
	buffer := &messageStreamBuffer{sessionID: "s", itemID: "c", kind: desktopstate.TimelineAssistant, index: -1}

	if got := timelineItemIndex(session, buffer); got != 2 || buffer.index != 2 {
		t.Fatalf("initial lookup = %d (cached %d), want 2", got, buffer.index)
	}

	// Prepend an item: the cached index stays in range but now points at a
	// different item, so the scan must repair it.
	session.Timeline = append([]desktopstate.TimelineItem{{ID: "z", Kind: desktopstate.TimelineUser}}, session.Timeline...)
	if got := timelineItemIndex(session, buffer); got != 3 || buffer.index != 3 {
		t.Fatalf("repaired lookup = %d (cached %d), want 3", got, buffer.index)
	}

	// Drop the front: the cached index falls out of range and is repaired.
	session.Timeline = session.Timeline[2:]
	if got := timelineItemIndex(session, buffer); got != 1 || buffer.index != 1 {
		t.Fatalf("shifted lookup = %d (cached %d), want 1", got, buffer.index)
	}

	// Item removed entirely.
	session.Timeline = nil
	if got := timelineItemIndex(session, buffer); got != -1 || buffer.index != -1 {
		t.Fatalf("missing lookup = %d (cached %d), want -1", got, buffer.index)
	}
}

func TestFlushMessageStreamUsesRepairedIndex(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	controller.state.Sessions[0].Timeline = []desktopstate.TimelineItem{
		{ID: "user-1", Kind: desktopstate.TimelineUser, Text: "hi"},
		{ID: "assistant-1", Kind: desktopstate.TimelineAssistant, Text: "before"},
	}
	key := messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}
	buffer := &messageStreamBuffer{sessionID: "session-1", itemID: "assistant-1", kind: desktopstate.TimelineAssistant, index: 1}
	buffer.text.WriteString("before after")
	controller.messageStreamBuffers[key] = buffer

	// Shift the timeline so the buffer's cached index is stale before the flush.
	controller.state.Sessions[0].Timeline = append([]desktopstate.TimelineItem{
		{ID: "note-0", Kind: desktopstate.TimelineStatus, Text: "note"},
	}, controller.state.Sessions[0].Timeline...)

	controller.mu.Lock()
	changed := controller.flushMessageStreamLocked(buffer)
	controller.mu.Unlock()
	if !changed {
		t.Fatal("flush against a shifted timeline reported no change")
	}

	var flushed *desktopstate.TimelineItem
	for index := range controller.state.Sessions[0].Timeline {
		if controller.state.Sessions[0].Timeline[index].ID == "assistant-1" {
			flushed = &controller.state.Sessions[0].Timeline[index]
		}
	}
	if flushed == nil || flushed.Text != "before after" || !flushed.Streaming {
		t.Fatalf("flushed item = %#v", flushed)
	}
	if buffer.index != 2 {
		t.Fatalf("buffer index after flush = %d, want 2", buffer.index)
	}
}

func TestUpsertTimelineItemLockedMergeAndAppend(t *testing.T) {
	session := &desktopstate.SessionState{Timeline: []desktopstate.TimelineItem{
		{ID: "assistant-1", Kind: desktopstate.TimelineAssistant, Text: "hello"},
	}}

	delta := upsertTimelineItemLocked(session, desktopstate.TimelineItem{
		ID: "assistant-1", Kind: desktopstate.TimelineAssistant, Text: " world",
	})
	if len(session.Timeline) != 1 || session.Timeline[0].Text != "hello world" {
		t.Fatalf("merged timeline = %#v", session.Timeline)
	}
	if want := len(" world"); delta != want {
		t.Fatalf("merge delta = %d, want %d", delta, want)
	}

	appended := desktopstate.TimelineItem{ID: "tool-1", Kind: desktopstate.TimelineTool, Title: "read", Status: "completed"}
	before := len(session.Timeline)
	if delta := upsertTimelineItemLocked(session, appended); delta != timelineItemSize(appended) {
		t.Fatalf("append delta = %d, want %d", delta, timelineItemSize(appended))
	}
	if len(session.Timeline) != before+1 || session.Timeline[before].ID != "tool-1" {
		t.Fatalf("appended timeline = %#v", session.Timeline)
	}
}

func TestIsSessionActive(t *testing.T) {
	for _, tc := range []struct {
		name      string
		state     desktopstate.State
		sessionID string
		agentID   string
		want      bool
	}{
		{
			name:      "exact match",
			state:     desktopstate.State{ActiveSessionID: "s1", ActiveAgentID: ProtonmanAgentID},
			sessionID: "s1",
			agentID:   ProtonmanAgentID,
			want:      true,
		},
		{
			name:      "empty active agent matches protonman",
			state:     desktopstate.State{ActiveSessionID: "s1", ActiveAgentID: ""},
			sessionID: "s1",
			agentID:   ProtonmanAgentID,
			want:      true,
		},
		{
			name:      "protonman active agent matches empty agent",
			state:     desktopstate.State{ActiveSessionID: "s1", ActiveAgentID: ProtonmanAgentID},
			sessionID: "s1",
			agentID:   "",
			want:      true,
		},
		{
			name:      "custom agent matches",
			state:     desktopstate.State{ActiveSessionID: "s1", ActiveAgentID: "reviewer"},
			sessionID: "s1",
			agentID:   "reviewer",
			want:      true,
		},
		{
			name:      "different session id fails",
			state:     desktopstate.State{ActiveSessionID: "s2", ActiveAgentID: ProtonmanAgentID},
			sessionID: "s1",
			agentID:   ProtonmanAgentID,
			want:      false,
		},
		{
			name:      "different agent id fails",
			state:     desktopstate.State{ActiveSessionID: "s1", ActiveAgentID: "reviewer"},
			sessionID: "s1",
			agentID:   ProtonmanAgentID,
			want:      false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSessionActive(tc.state, tc.sessionID, tc.agentID); got != tc.want {
				t.Fatalf("isSessionActive = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFinishSessionHistoryLoadCancelledDoesNotSetStatus(t *testing.T) {
	controller := newTestController()
	current := controller.clients[ProtonmanAgentID]
	controller.statuses[ProtonmanAgentID] = "Connected"
	controller.histories["session-1"] = HistoryStateLoading

	controller.finishSessionHistoryLoad(current, "session-1", context.Canceled)

	if got := controller.histories["session-1"]; got != HistoryStateUnloaded {
		t.Fatalf("history state = %v, want unloaded", got)
	}
	if status := controller.statuses[ProtonmanAgentID]; status != "Connected" {
		t.Fatalf("status = %q, want Connected (should not report failure on cancellation)", status)
	}
}

func TestFinishSessionHistoryInactiveSessionFailureDoesNotCorruptActiveStatus(t *testing.T) {
	controller := newTestController()
	current := controller.clients[ProtonmanAgentID]
	controller.state.ActiveSessionID = "session-2"
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{
		ID:      "session-2",
		AgentID: ProtonmanAgentID,
		Status:  desktopstate.TaskIdle,
	})
	controller.statuses[ProtonmanAgentID] = "Connected"
	controller.histories["session-1"] = HistoryStateLoading

	controller.finishSessionHistoryLoad(current, "session-1", errors.New("network failure"))

	if got := controller.histories["session-1"]; got != HistoryStateUnloaded {
		t.Fatalf("session-1 history state = %v, want unloaded", got)
	}
	if status := controller.statuses[ProtonmanAgentID]; status != "Connected" {
		t.Fatalf("active session status = %q, want Connected (inactive session error should not corrupt active session status)", status)
	}
}

func TestFindWorkspaceForSession(t *testing.T) {
	dir := t.TempDir()
	wsKey := sessionWorkspaceKey(dir)

	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{
			{
				ID: "project-1",
				Folders: []desktopstate.ProjectFolder{
					{Path: dir, Primary: true},
				},
			},
		},
	}

	// Preserves existing workspace
	sessWithWs := desktopstate.SessionState{ID: "s1", Workspace: "/custom/path"}
	if got := findWorkspaceForSession(state, sessWithWs); got != "/custom/path" {
		t.Fatalf("got %q, want /custom/path", got)
	}

	// Finds matching folder by session WorkspaceKey
	sessWithKey := desktopstate.SessionState{ID: "s2", WorkspaceKey: wsKey}
	if got := findWorkspaceForSession(state, sessWithKey); got != dir {
		t.Fatalf("got %q, want %q", got, dir)
	}

	// Does not force mismatching default workspace
	sessWithOtherKey := desktopstate.SessionState{ID: "s3", WorkspaceKey: "0123456789abcdef"}
	if got := findWorkspaceForSession(state, sessWithOtherKey); got != "" {
		t.Fatalf("got %q, want empty string for mismatching workspace key", got)
	}
}
