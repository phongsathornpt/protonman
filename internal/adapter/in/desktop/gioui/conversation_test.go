//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
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
		ctx:                 context.Background(),
		state:               desktopstate.State{ActiveSessionID: "session-1", Sessions: []desktopstate.SessionState{{ID: "session-1", AgentID: controllerAgentID, Status: desktopstate.TaskIdle}}},
		profiles:            map[string]app.ACPAgentProfile{controllerAgentID: profile},
		clients:             map[string]*acpclient.Client{controllerAgentID: client},
		connections:         map[string]connectionPhase{controllerAgentID: connectionConnected},
		statuses:            map[string]string{controllerAgentID: "Connected"},
		activeAgentID:       controllerAgentID,
		histories:           make(map[string]historyState),
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
	prior := controller.snapshot()

	key := messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}
	buffer := &messageStreamBuffer{sessionID: key.sessionID, itemID: "assistant-1", kind: desktopstate.TimelineAssistant}
	buffer.text.WriteString("after")
	controller.messageStreamBuffers[key] = buffer
	controller.flushMessageStream(key, buffer)

	current := controller.snapshot()
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

func TestSelectSessionEvictsInactiveTimeline(t *testing.T) {
	controller := newTestController()
	controller.clients = nil
	controller.state.ActiveSessionID = "session-1"
	controller.state.Sessions = append(controller.state.Sessions, desktopstate.SessionState{
		ID: "session-2", AgentID: controllerAgentID,
		Timeline:  []desktopstate.TimelineItem{{ID: "old", Kind: desktopstate.TimelineAssistant, Text: "cached history"}},
		Subagents: []desktopstate.SubagentState{{ID: "child", Summary: "cached result"}},
		Context:   desktopstate.SessionContextState{Goal: "cached goal", Memory: desktopstate.MemoryState{Workspace: []desktopstate.MemoryEntryState{{Value: "cached memory"}}}},
		Runtime:   desktopstate.RuntimeSettingsState{Model: "cached model"},
	})
	controller.state.Sessions[0].Timeline = []desktopstate.TimelineItem{{ID: "current", Kind: desktopstate.TimelineAssistant, Text: "active history"}}
	controller.histories["session-1"] = historyStateLoaded
	controller.histories["session-2"] = historyStateLoaded
	controller.messageStreams[messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}] = "stream"

	controller.selectSession("session-2")

	if len(controller.state.Sessions[0].Timeline) != 0 {
		t.Fatalf("inactive transcript retained %d items", len(controller.state.Sessions[0].Timeline))
	}
	if len(controller.state.Sessions[0].Subagents) != 0 {
		t.Fatalf("inactive subagent history retained %d items", len(controller.state.Sessions[0].Subagents))
	}
	if controller.state.Sessions[0].Context.Goal != "" || len(controller.state.Sessions[0].Context.Memory.Workspace) != 0 || controller.state.Sessions[0].Runtime.Model != "" {
		t.Fatal("inactive session inspector state was retained")
	}
	if controller.histories["session-1"] != historyStateUnloaded {
		t.Fatalf("inactive history state = %v, want unloaded", controller.histories["session-1"])
	}
	if controller.state.Sessions[1].Timeline[0].Text != "cached history" {
		t.Fatal("selected session history was evicted")
	}
	if _, ok := controller.messageStreams[messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}]; ok {
		t.Fatal("inactive stream retained")
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
		ID: "session-2", AgentID: controllerAgentID,
	})
	controller.histories["session-2"] = historyStateLoading

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
		ID: "session-2", AgentID: controllerAgentID,
	})
	controller.histories["session-2"] = historyStateLoading
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
	controller.pruneInactiveSessionHistoryLocked("session-1")
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
	if controller.histories["session-2"] != historyStateUnloaded {
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
	controller.histories["session-1"] = historyStateLoaded
	controller.histories["removed"] = historyStateLoaded
	controller.historyLoads = make(map[string]*sessionHistoryLoad)
	removedLoadCtx, cancelRemovedLoad := context.WithCancel(context.Background())
	controller.historyLoads["removed"] = &sessionHistoryLoad{cancel: cancelRemovedLoad}
	controller.historyStaging["removed"] = []desktopstate.Event{{Kind: desktopstate.EventTimelineAppended}}
	controller.messageSequence["removed"] = 4
	controller.messageSequence["session-1"] = 2
	controller.messageStreams[messageStreamKey{sessionID: "removed", kind: "agent_message_chunk"}] = "old-stream"
	controller.messageStreams[messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}] = "live-stream"
	controller.permissionWait["live-permission"] = make(chan string, 1)
	removedWaiter := make(chan string, 1)
	controller.permissionWait["removed-permission"] = removedWaiter
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
	if _, ok := controller.permissionWait["removed-permission"]; ok {
		t.Fatal("orphaned permission waiter was retained")
	}
	if outcome := <-removedWaiter; outcome != "" {
		t.Fatalf("orphaned permission outcome = %q, want cancellation", outcome)
	}
	if controller.permissionWait["live-permission"] == nil {
		t.Fatal("live permission waiter was pruned")
	}
	if controller.runtimeMutation != "" {
		t.Fatalf("deleted session runtime mutation = %q", controller.runtimeMutation)
	}
}

func TestSessionUpdatesRouteThroughOwningAgent(t *testing.T) {
	controller := newTestController()
	reviewerClient := &acpclient.Client{}
	controller.clients["reviewer"] = reviewerClient
	controller.connections["reviewer"] = connectionConnected
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

func TestFinishSessionHistoryAppliesStagedEvents(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	controller.contextRefresh = &sessionRefreshTracker{inFlight: map[string]bool{"session-1": true}}
	controller.memoryRefresh = &sessionRefreshTracker{inFlight: map[string]bool{"session-1": true}}
	controller.runtimeRefresh = &sessionRefreshTracker{inFlight: map[string]bool{"session-1": true}}
	client := controller.clients[controllerAgentID]
	controller.histories["session-1"] = historyStateLoading
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

	if got := controller.histories["session-1"]; got != historyStateLoaded {
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
	controller.histories["session-1"] = historyStateLoading
	controller.stageHistoryEventLocked("session-1", desktopstate.Event{
		Kind: desktopstate.EventTimelineAppended, SessionID: "session-1",
		Item: desktopstate.TimelineItem{Kind: desktopstate.TimelineAssistant, Text: strings.Repeat("x", maxHistoryStagingBytes)},
	})
	controller.stageHistoryEventLocked("session-1", desktopstate.Event{
		Kind: desktopstate.EventTimelineAppended, SessionID: "session-1",
		Item: desktopstate.TimelineItem{Kind: desktopstate.TimelineAssistant, Text: "kept recent"},
	})

	controller.finishSessionHistoryLoad(controller.clients[controllerAgentID], "session-1", nil)

	if controller.histories["session-1"] != historyStateLoaded {
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
