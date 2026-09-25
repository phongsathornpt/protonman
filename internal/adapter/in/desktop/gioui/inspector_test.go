//go:build desktop || desktop_gio

package gioui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestSessionRefreshTrackerThrottlesIndependently(t *testing.T) {
	tracker := newSessionRefreshTracker(750 * time.Millisecond)
	now := time.Unix(10, 0)
	if !tracker.begin("one", false, now) {
		t.Fatal("first refresh was rejected")
	}
	if tracker.begin("one", false, now.Add(time.Second)) {
		t.Fatal("in-flight refresh was not rejected")
	}
	tracker.finish("one", now)
	if tracker.begin("one", false, now.Add(500*time.Millisecond)) {
		t.Fatal("refresh was not throttled")
	}
	if !tracker.begin("one", false, now.Add(750*time.Millisecond)) {
		t.Fatal("refresh was not admitted after the interval")
	}
	if !tracker.begin("two", false, now) {
		t.Fatal("second session was incorrectly throttled")
	}
	tracker.reset()
	if !tracker.begin("one", false, now) {
		t.Fatal("reset did not admit a new refresh")
	}
}

func TestProjectSessionInspectorPayloads(t *testing.T) {
	contextResult := sessionContextResult{SessionID: " session-1 ", Goal: " ship "}
	contextResult.Todo.Revision = 4
	contextResult.Todo.Items = append(contextResult.Todo.Items, struct {
		ID     string `json:"id"`
		Text   string `json:"text"`
		Status string `json:"status"`
	}{ID: " item-1 ", Text: " verify ", Status: " in_progress "})
	contextState := projectSessionContext(contextResult)
	if contextState.Goal != "ship" || contextState.Todo.Revision != 4 || len(contextState.Todo.Items) != 1 {
		t.Fatalf("context projection = %+v", contextState)
	}
	if contextState.Todo.Items[0].ID != "item-1" || contextState.Todo.Items[0].Text != "verify" || contextState.Todo.Items[0].Status != "in_progress" {
		t.Fatalf("context item projection = %+v", contextState.Todo.Items[0])
	}

	memoryResult := sessionMemoryResult{SessionID: "session-1", WorkspaceKey: " workspace-1 "}
	memoryResult.Workspace = append(memoryResult.Workspace, struct {
		ID         string  `json:"id"`
		Scope      string  `json:"scope"`
		Kind       string  `json:"kind"`
		Key        string  `json:"key"`
		Value      string  `json:"value"`
		Confidence float64 `json:"confidence"`
		UsageCount uint64  `json:"usageCount"`
	}{ID: " m1 ", Scope: " workspace ", Kind: " repo_fact ", Key: " test ", Value: " go test ", Confidence: .9, UsageCount: 3})
	memory := projectSessionMemory(memoryResult)
	if memory.WorkspaceKey != "workspace-1" || len(memory.Workspace) != 1 {
		t.Fatalf("memory projection = %+v", memory)
	}
	if memory.Workspace[0].ID != "m1" || memory.Workspace[0].Value != "go test" || memory.Workspace[0].UsageCount != 3 {
		t.Fatalf("memory entry projection = %+v", memory.Workspace[0])
	}

	runtime := projectSessionRuntime(sessionRuntimeResult{
		SessionID: "session-1", Provider: " openai ", Model: " gpt ", Reasoning: " high ", LowConcurrency: " on ",
	})
	if runtime.Provider != "openai" || runtime.Model != "gpt" || runtime.Reasoning != "high" || runtime.LowConcurrency != "on" {
		t.Fatalf("runtime projection = %+v", runtime)
	}
}

func TestInspectorResultsRejectStaleClient(t *testing.T) {
	controller := newTestController()
	current := controller.clients[controllerAgentID]
	if _, _, ok := controller.beginSessionRefresh("session-1", true, contextRefreshKind, &acpclient.Client{}); ok {
		t.Fatal("stale client was admitted for inspector refresh")
	}
	controller.state.Sessions[0].Context.Memory = desktopstate.MemoryState{Global: []desktopstate.MemoryEntryState{{ID: "keep"}}}
	stale := &acpclient.Client{}
	if controller.applySessionContext(stale, sessionContextResult{SessionID: "session-1", Goal: "stale"}) {
		t.Fatal("stale context result was applied")
	}
	if controller.applySessionMemory(stale, sessionMemoryResult{SessionID: "session-1", WorkspaceKey: "stale"}) {
		t.Fatal("stale memory result was applied")
	}
	if controller.applySessionRuntime(stale, sessionRuntimeResult{SessionID: "session-1", Model: "stale"}) {
		t.Fatal("stale runtime result was applied")
	}
	if !controller.applySessionRuntime(current, sessionRuntimeResult{SessionID: "session-1", Provider: "openai", Model: "gpt", Reasoning: "high", LowConcurrency: "on"}) {
		t.Fatal("current runtime result was rejected")
	}
	if !controller.applySessionContext(current, sessionContextResult{SessionID: "session-1", Goal: "current"}) {
		t.Fatal("current context result was rejected")
	}
	if got := controller.state.Sessions[0].Context.Memory.Global; len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("context update replaced memory: %+v", got)
	}
	if got := controller.state.Sessions[0].Runtime; got.Model != "gpt" || got.Reasoning != "high" {
		t.Fatalf("runtime state = %+v", got)
	}
}

func TestSyncRuntimeEditorsPreservesUserDraft(t *testing.T) {
	view := newShell(newTheme("light"))
	state := desktopstate.State{
		ActiveSessionID: "session-1",
		Sessions: []desktopstate.SessionState{{
			ID:      "session-1",
			Runtime: desktopstate.RuntimeSettingsState{Provider: "openai", Model: "gpt"},
		}},
	}
	view.syncRuntimeEditors(state)
	view.runtimeProviderEditor.SetText("draft-provider")
	view.syncRuntimeEditors(state)
	if got := view.runtimeProviderEditor.Text(); got != "draft-provider" {
		t.Fatalf("provider editor was overwritten: %q", got)
	}
	state.Sessions[0].Runtime.Model = "gpt-next"
	view.syncRuntimeEditors(state)
	if got := view.runtimeModelEditor.Text(); got != "gpt-next" {
		t.Fatalf("updated model was not synchronized: %q", got)
	}
}

func TestRuntimeMutationRejectsBusyOrDuplicateRequests(t *testing.T) {
	controller := newTestController()
	controller.state.ActiveSessionID = "session-1"
	controller.state.Sessions[0].Status = desktopstate.TaskRunning
	controller.setRuntimeReasoning("high")
	if controller.runtimeMutation != "" {
		t.Fatal("busy session accepted a runtime mutation")
	}

	controller.state.Sessions[0].Status = desktopstate.TaskIdle
	controller.runtimeMutation = "other-session"
	controller.setRuntimeLowConcurrency("on")
	if controller.runtimeMutation != "other-session" {
		t.Fatalf("duplicate mutation replaced active request: %q", controller.runtimeMutation)
	}
}

func TestTodoMarkerForStatus(t *testing.T) {
	for status, want := range map[string]string{
		"completed":   "✓",
		"in_progress": "◐",
		"pending":     "○",
	} {
		if got := todoMarkerForStatus(status); got != want {
			t.Fatalf("marker for %q = %q, want %q", status, got, want)
		}
	}
}

func TestCompactInspectorTextIsUTF8Safe(t *testing.T) {
	got := compactInspectorText(strings.Repeat("ก", 20), 8)
	if !utf8.ValidString(got) || len([]rune(got)) != 8 {
		t.Fatalf("compacted text = %q", got)
	}
}
