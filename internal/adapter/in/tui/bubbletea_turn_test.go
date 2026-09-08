// Code grouped by TUI behavior boundary; shared fixtures live in bubbletea_helpers_test.go.
package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	domainmodel "github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"

	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	"strings"
	"testing"
	"time"
)

func TestBubbleModelRunsToolCommandThroughService(t *testing.T) {
	registry, handler := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	model := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		newPermissionBridge(),
		"",
	)
	model.resize(80, 24)
	model.prompt.SetValue(`:call read_file {"path":"README.md"}`)
	command := model.submit()
	if command == nil {
		t.Fatal("submit() command = nil, want tool command")
	}
	message := command()
	resultMessage, ok := message.(toolResultMsg)
	if !ok {
		t.Fatalf("tool command message = %T, want toolResultMsg", message)
	}
	updated, _ := model.Update(resultMessage)
	model = updated.(*bubbleModel)
	if handler.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", handler.calls)
	}
	if !strings.Contains(plainTranscript(model), "file contents") {
		t.Fatalf("scrollback does not contain tool output: %#v", model.blocks)
	}
}

func TestSubmitWhileBusyQueuesDraft(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	model.resize(80, 24)
	model.busy = true
	model.prompt.SetValue(":help")

	if command := model.submit(); command != nil {
		t.Fatalf("busy submit command = %v, want nil", command)
	}
	if got := model.prompt.Value(); got != "" {
		t.Fatalf("busy submit cleared prompt = %q, want empty", got)
	}
	if len(model.queue) != 1 || model.queue[0] != ":help" {
		t.Fatalf("queue = %#v, want [:help]", model.queue)
	}
	if !strings.Contains(plainTranscript(model), "queued (1): :help") {
		t.Fatalf("scrollback missing queue notice: %#v", model.blocks)
	}

	model.busy = false
	if command := model.drainQueue(); command != nil {
		t.Fatalf("queued :help command = %v, want nil", command)
	}
	if len(model.queue) != 0 {
		t.Fatalf("queue after drain = %#v, want empty", model.queue)
	}
	if !strings.Contains(plainTranscript(model), "/help") {
		t.Fatalf("drained :help did not render: %#v", model.blocks)
	}
}

func TestAppendTurnResultCoalescesAssistantText(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.appendTurnResult(
		[]applicationturn.Event{
			{Kind: applicationturn.EventTextDelta, Text: "hello"},
			{Kind: applicationturn.EventTextDelta, Text: " world"},
		},
		applicationturn.Result{Message: domainmodel.Message{Content: "hello world"}},
		nil,
	)

	plain := plainTranscript(model)
	if strings.Count(plain, "hello world") != 1 {
		t.Fatalf("assistant text count = %d, want 1: %q", strings.Count(plain, "hello world"), plain)
	}
	if strings.Contains(plain, "assistant: hello") {
		t.Fatalf("text deltas were not coalesced: %q", plain)
	}
}

func TestAppendTurnResultPreservesToolNewlines(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.appendTurnResult(
		[]applicationturn.Event{
			{
				Kind: applicationturn.EventToolResult,
				Call: tool.Call{Name: "read_file"},
				Result: tool.Result{
					Output: "alpha\nbeta\ngamma",
				},
			},
		},
		applicationturn.Result{},
		nil,
	)

	plain := plainTranscript(model)
	if !strings.Contains(plain, "alpha\nbeta\ngamma") {
		t.Fatalf("tool output newlines were flattened: %q", plain)
	}
}

func TestStartTurnStreamsSinkEvents(t *testing.T) {
	runner := &scriptedRunner{
		events: []applicationturn.Event{
			{Kind: applicationturn.EventTextDelta, Text: "hello"},
			{Kind: applicationturn.EventTextDelta, Text: " stream"},
		},
		result: applicationturn.Result{
			Message: domainmodel.Message{Role: domainmodel.RoleAssistant, Content: "hello stream"},
		},
	}
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	model := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		runner,
		newPermissionBridge(),
		"",
	)
	if command := model.startTurn("hi"); command == nil {
		t.Fatal("startTurn command = nil")
	}
	deadline := time.Now().Add(time.Second)
	for model.busy {
		if time.Now().After(deadline) {
			t.Fatal("streamed turn did not finish")
		}
		events := model.turnEvents
		if events == nil {
			t.Fatal("busy turn has no event channel")
		}
		message := waitTurnCh(events)()
		updated, _ := model.Update(message)
		model = updated.(*bubbleModel)
	}
	plain := plainTranscript(model)
	if !strings.Contains(plain, "hello stream") {
		t.Fatalf("streamed transcript = %q, want hello stream", plain)
	}
	if model.busy {
		t.Fatal("model still busy after streamed turn")
	}
}

func TestClosedTurnEventsRenderTerminalFailure(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.busy = true
	events := make(chan tea.Msg)
	close(events)
	model.turnEvents = events

	updated, _ := model.Update(turnEventsClosedMsg{})
	model = updated.(*bubbleModel)

	if model.busy {
		t.Fatal("model remained busy after turn event channel closed")
	}
	if !strings.Contains(plainTranscript(model), "turn event stream closed before completion") {
		t.Fatalf("closed turn channel missing terminal failure: %q", plainTranscript(model))
	}
}

func TestTurnDoneAppendsProducedToolHistory(t *testing.T) {
	runner := &scriptedRunner{result: applicationturn.Result{
		Message: domainmodel.Message{Role: domainmodel.RoleAssistant, Content: "done"},
		Messages: []domainmodel.Message{
			{Role: domainmodel.RoleAssistant, ToolCalls: []domainmodel.ToolCall{{ID: "call-1", Name: "read_file", Arguments: []byte(`{"path":"README.md"}`)}}},
			{Role: domainmodel.RoleTool, ToolCallID: "call-1", ToolName: "read_file", Content: `{"output":"ok"}`},
			{Role: domainmodel.RoleAssistant, Content: "done"},
		},
	}}
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	m := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), runner, newPermissionBridge(), "")

	message := m.startTurn("inspect")()
	updated, _ := m.Update(message)
	m = updated.(*bubbleModel)

	if got, want := len(m.messages), 4; got != want {
		t.Fatalf("provider history length = %d, want %d", got, want)
	}
	if m.messages[1].Role != domainmodel.RoleAssistant || m.messages[2].Role != domainmodel.RoleTool {
		t.Fatalf("provider history = %#v, want assistant/tool exchange", m.messages)
	}
}

func TestBangPrefixSubmitsBashCall(t *testing.T) {
	registry := newNamedTestRegistry(tool.Definition{
		Name:                "bash",
		Description:         "run a shell command",
		Kind:                tool.KindBash,
		PermissionDetailKey: "command",
	})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	model := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		newPermissionBridge(),
		"",
	)
	model.setBashMode(true)
	model.prompt.SetValue("pwd")
	command := model.submit()
	if command == nil {
		t.Fatal("bash submit command = nil")
	}
	if model.bottom.bashMode() {
		t.Fatal("bash mode stayed on after submit")
	}
	message := command()
	resultMessage, ok := message.(toolResultMsg)
	if !ok {
		t.Fatalf("bash message = %T, want toolResultMsg", message)
	}
	if resultMessage.err != nil {
		t.Fatalf("bash call error = %v", resultMessage.err)
	}
	if registry.handler.calls != 1 {
		t.Fatalf("bash handler calls = %d, want 1", registry.handler.calls)
	}
}

func TestTodoStoreRevisionSyncsAfterToolResult(t *testing.T) {
	initial := []TodoItem{{ID: "a", Text: "inspect", Status: tododomain.StatusPending}}
	store, err := tododomain.NewStore(initial)
	if err != nil {
		t.Fatal(err)
	}
	m := newTestBubbleModel(t, permission.ModeAsk, initial)
	m.todoStore = store
	m.todoRevision = store.Snapshot().Revision
	if _, err := store.Replace(context.Background(), []tododomain.Item{{ID: "a", Text: "inspect", Status: tododomain.StatusCompleted}}); err != nil {
		t.Fatal(err)
	}
	m.applyTurnEvent(applicationturn.Event{Kind: applicationturn.EventToolResult, Call: tool.Call{ID: "todo-1", Name: "update_todo"}, Result: tool.Result{CallID: "todo-1", ToolName: "update_todo"}})
	if len(m.todo) != 1 || m.todo[0].Status != tododomain.StatusCompleted {
		t.Fatalf("todo = %#v", m.todo)
	}
	if m.todoRevision != store.Snapshot().Revision {
		t.Fatalf("revision = %d", m.todoRevision)
	}
	if m.syncTodoSnapshot() {
		t.Fatal("unchanged revision reported a sync")
	}
}
