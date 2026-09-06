package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	domainmodel "github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
)

func TestPlanModeBlocksBashBeforeAlwaysApprove(t *testing.T) {
	handler := &countingHandler{definition: tool.Definition{
		Name:                "bash",
		Description:         "run shell command",
		Kind:                tool.KindBash,
		PermissionDetailKey: "command",
	}}
	registry := behaviorRegistry{handler: handler}
	service := newBehaviorService(t, registry, permission.ModeAlwaysApprove)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")

	model.setPlanMode("on")
	if !model.planMode {
		t.Fatal("plan mode was not enabled")
	}
	if service.Mode() != permission.ModeAsk {
		t.Fatalf("plan mode left service mode = %s, want ask", service.Mode())
	}
	if !strings.Contains(model.modeChip(), "read-only") {
		t.Fatalf("plan chip does not communicate read-only behavior: %q", model.modeChip())
	}

	call, err := tool.NewCall("plan-bash", "bash", []byte(`{"command":"rm -f tmp"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	message := model.startTool(call)()
	resultMessage, ok := message.(toolResultMsg)
	if !ok {
		t.Fatalf("tool message = %T, want toolResultMsg", message)
	}
	if !errors.Is(resultMessage.err, toolcall.ErrPermissionDenied) {
		t.Fatalf("plan bash error = %v, want permission denied", resultMessage.err)
	}
	if !resultMessage.result.Denied {
		t.Fatal("plan bash result was not marked denied")
	}
	if handler.calls != 0 {
		t.Fatalf("plan mode executed bash handler %d times", handler.calls)
	}

	updated, _ := model.Update(resultMessage)
	model = updated.(*bubbleModel)
	assertNoRunningTool(t, model)
	if !strings.Contains(plainTranscript(model), "plan mode is read-only") {
		t.Fatalf("plan denial missing from transcript: %q", plainTranscript(model))
	}
}

func TestCtrlCCancelsDirectToolWithoutQuitting(t *testing.T) {
	handler := &blockingHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "run shell command",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
		started: make(chan struct{}),
	}
	registry := behaviorRegistry{handler: handler}
	service := newBehaviorService(t, registry, permission.ModeAlwaysApprove)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	call, err := tool.NewCall("cancel-bash", "bash", []byte(`{"command":"sleep 30"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}

	command := model.startTool(call)
	messageCh := make(chan tea.Msg, 1)
	go func() { messageCh <- command() }()
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("tool did not start")
	}

	updated, cancelCommand := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(*bubbleModel)
	if cancelCommand != nil {
		t.Fatal("ctrl+c while a tool is active should cancel, not quit")
	}

	var message tea.Msg
	select {
	case message = <-messageCh:
	case <-time.After(time.Second):
		t.Fatal("tool did not return after cancellation")
	}
	resultMessage := message.(toolResultMsg)
	if !errors.Is(resultMessage.err, context.Canceled) {
		t.Fatalf("tool cancellation error = %v, want context canceled", resultMessage.err)
	}
	updated, _ = model.Update(resultMessage)
	model = updated.(*bubbleModel)
	if model.busy {
		t.Fatal("model stayed busy after direct tool cancellation")
	}
	if model.turnCancel != nil {
		t.Fatal("active cancel function remained after tool cancellation")
	}
	assertNoRunningTool(t, model)
	plain := plainTranscript(model)
	if !strings.Contains(plain, "cancelled") {
		t.Fatalf("cancelled tool missing neutral terminal state: %q", plain)
	}
}

func TestTurnCancellationRendersNeutralTerminalState(t *testing.T) {
	runner := &blockingRunner{started: make(chan struct{})}
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{
		Name:        "read_file",
		Description: "read file",
		Kind:        tool.KindRead,
	}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, runner, newPermissionBridge(), "")

	command := model.startTurn("wait")
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("turn did not start")
	}
	updated, cancelCommand := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(*bubbleModel)
	if cancelCommand != nil {
		t.Fatal("ctrl+c while a turn is active should cancel, not quit")
	}

	message := command()
	updated, _ = model.Update(message)
	model = updated.(*bubbleModel)
	plain := plainTranscript(model)
	if !strings.Contains(plain, "turn cancelled") {
		t.Fatalf("turn cancellation missing from transcript: %q", plain)
	}
	if strings.Contains(plain, "turn failed") {
		t.Fatalf("user cancellation rendered as failure: %q", plain)
	}
	if model.busy || model.turnCancel != nil {
		t.Fatalf("turn cancellation left active state: busy=%v cancel=%v", model.busy, model.turnCancel != nil)
	}
}

func TestTurnWorkerPanicRendersTerminalFailure(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{
		Name:        "read_file",
		Description: "read file",
		Kind:        tool.KindRead,
	}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(
		context.Background(),
		service,
		registry,
		nil,
		panicRunner{},
		newPermissionBridge(),
		"",
	)

	message := model.startTurn("panic")()
	updated, _ := model.Update(message)
	model = updated.(*bubbleModel)

	if model.busy {
		t.Fatal("model remained busy after turn worker panic")
	}
	if !strings.Contains(plainTranscript(model), "turn worker panicked") {
		t.Fatalf("panic reason missing from transcript: %q", plainTranscript(model))
	}
}

func TestTurnFailureFinalizesRunningToolCells(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	call, err := tool.NewCall("cancel-tool", "read_file", []byte(`{"path":"README.md"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	model.appendToolCall(call)
	model.busy = true

	updated, _ := model.Update(turnDoneMsg{err: context.Canceled})
	model = updated.(*bubbleModel)

	assertNoRunningTool(t, model)
	if !strings.Contains(plainTranscript(model), "cancelled") {
		t.Fatalf("cancelled tool missing terminal state: %q", plainTranscript(model))
	}
}

func TestCompletedTurnSyncsLegacyAssistantBlock(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.applyTurnEvent(applicationturn.Event{Kind: applicationturn.EventTextDelta, Text: "streamed answer"})
	model.applyTurnEvent(applicationturn.Event{Kind: applicationturn.EventCompleted})
	if len(model.blocks) != 1 {
		t.Fatalf("completed turn legacy blocks = %#v, want one assistant block", model.blocks)
	}
	if got := model.blocks[0]; got.Kind != blockAssistant || got.Body != "streamed answer" {
		t.Fatalf("completed assistant block = %#v", got)
	}
}

func TestFailedToolReplacesRunningBlock(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.appendToolRunning("bash")
	result := tool.Result{
		ToolName: "bash",
		Failure: &tool.Failure{
			Code:    tool.ErrorCodePermissionDenied,
			Message: "permission denied",
		},
	}
	model.appendToolResult(result, toolcall.ErrPermissionDenied)

	assertNoRunningTool(t, model)
	if len(model.blocks) != 1 {
		t.Fatalf("failure left duplicate blocks: %#v", model.blocks)
	}
	if model.blocks[0].Kind != blockError {
		t.Fatalf("failed running block kind = %v, want error", model.blocks[0].Kind)
	}
}

func TestModelToolFailureRendersReason(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	call, err := tool.NewCall("denied-tool", "read_file", []byte(`{"path":".env"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	model.appendToolCall(call)
	model.applyTurnEvent(applicationturn.Event{
		Kind: applicationturn.EventToolResult,
		Call: call,
		Result: tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Denied:   true,
			Failure: &tool.Failure{
				Code:    tool.ErrorCodePermissionDenied,
				Message: "blocked by workspace policy",
			},
		},
		Err: toolcall.ErrPermissionDenied,
	})

	plain := plainTranscript(model)
	if !strings.Contains(plain, "blocked by workspace policy") {
		t.Fatalf("tool failure reason missing from transcript: %q", plain)
	}
	assertNoRunningTool(t, model)
}

func assertNoRunningTool(t *testing.T, model *bubbleModel) {
	t.Helper()
	for _, block := range model.blocks {
		if block.Kind == blockTool && block.Running {
			t.Fatalf("running tool block remained after terminal result: %#v", model.blocks)
		}
	}
}

type behaviorRegistry struct {
	handler tool.Handler
}

func (r behaviorRegistry) Lookup(name string) (tool.Handler, bool) {
	if r.handler == nil || r.handler.Definition().Name != name {
		return nil, false
	}
	return r.handler, true
}

func (r behaviorRegistry) Definitions() []tool.Definition {
	if r.handler == nil {
		return nil
	}
	return []tool.Definition{r.handler.Definition()}
}

type countingHandler struct {
	definition tool.Definition
	calls      int
}

func (h *countingHandler) Definition() tool.Definition { return h.definition }

func (h *countingHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls++
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: "ok"}, nil
}

type blockingHandler struct {
	definition tool.Definition
	started    chan struct{}
}

func (h *blockingHandler) Definition() tool.Definition { return h.definition }

func (h *blockingHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	close(h.started)
	<-ctx.Done()
	return tool.Result{CallID: call.ID, ToolName: call.Name}, ctx.Err()
}

type blockingRunner struct {
	started chan struct{}
}

type panicRunner struct{}

func (panicRunner) Run(context.Context, []domainmodel.Message, applicationturn.Sink) (applicationturn.Result, error) {
	panic("test runner panic")
}

func (r *blockingRunner) Run(
	ctx context.Context,
	_ []domainmodel.Message,
	_ applicationturn.Sink,
) (applicationturn.Result, error) {
	close(r.started)
	<-ctx.Done()
	return applicationturn.Result{}, ctx.Err()
}

func newBehaviorService(t *testing.T, registry tool.Registry, mode permission.Mode) *toolcall.Service {
	t.Helper()
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(mode))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestSpinnerTickSkipsViewportRefreshForStreamingAssistant(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(80, 24)
	m.busy = true
	m.historyState.AppendAssistantDelta("streaming assistant text")
	m.viewport.SetContent("viewport sentinel")
	updated, _ := m.Update(spinner.TickMsg{})
	m = updated.(*bubbleModel)
	if got := m.viewport.View(); !strings.Contains(got, "viewport sentinel") {
		t.Fatalf("assistant-only spinner tick refreshed viewport: %q", got)
	}
}

func TestViewportTailOnlyHydratesBeforePageUp(t *testing.T) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	m.resize(80, 18)
	m.showWelcome = false
	m.busy = true
	m.followTail = true
	for i := 0; i < 40; i++ {
		m.historyState.Append(&AssistantCell{Text: fmt.Sprintf("answer %d\nmore detail", i)})
	}
	m.historyState.AppendAssistantDelta("live one\nlive two\nlive three")
	m.refreshViewport()
	if !m.viewportTailOnly {
		t.Fatal("expected streaming follow-tail viewport to use bounded tail content")
	}
	tailLines := m.viewport.TotalLineCount()
	if tailLines > m.viewport.Height {
		t.Fatalf("tail viewport has %d lines, height %d", tailLines, m.viewport.Height)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = updated.(*bubbleModel)
	if m.viewportTailOnly {
		t.Fatal("page up should hydrate full scrollback")
	}
	if m.viewport.TotalLineCount() <= tailLines {
		t.Fatalf("expected hydrated viewport to restore scrollback: tail=%d full=%d", tailLines, m.viewport.TotalLineCount())
	}
	if m.viewport.AtBottom() {
		t.Fatal("page up should leave the viewport above the tail")
	}
}

func TestPlanModeAllowsProvenReadOnlyBash(t *testing.T) {
	handler := &countingHandler{definition: tool.Definition{
		Name: "bash", Description: "run shell command", Kind: tool.KindBash, PermissionDetailKey: "command",
	}}
	registry := behaviorRegistry{handler: handler}
	service := newBehaviorService(t, registry, permission.ModeAlwaysApprove)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	model.setPlanEnabled(true)
	call, err := tool.NewCall("plan-read-bash", "bash", []byte(`{"command":"pwd && git status --short"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Call(context.Background(), call); err != nil {
		t.Fatalf("read-only bash blocked in plan mode: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", handler.calls)
	}
}

func TestPlanModeBlocksUnknownBash(t *testing.T) {
	handler := &countingHandler{definition: tool.Definition{Name: "bash", Description: "run shell command", Kind: tool.KindBash, PermissionDetailKey: "command"}}
	registry := behaviorRegistry{handler: handler}
	service := newBehaviorService(t, registry, permission.ModeAlwaysApprove)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	model.setPlanEnabled(true)
	call, _ := tool.NewCall("plan-unknown-bash", "bash", []byte(`{"command":"go test ./..."}`))
	if _, err := service.Call(context.Background(), call); !errors.Is(err, toolcall.ErrPermissionDenied) {
		t.Fatalf("unknown bash error = %v, want permission denied", err)
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls = %d, want 0", handler.calls)
	}
}

func TestTodoConflictRendersTaskSpecificGuidance(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	call, _ := tool.NewCall("todo-conflict", "update_todo", []byte(`{"expected_revision":1,"items":[]}`))
	m.appendToolCall(call)
	m.applyToolResult("update_todo", tool.Result{
		CallID: "todo-conflict", ToolName: "update_todo",
		Failure: &tool.Failure{Code: tool.ErrorCodeConflict, Message: "todo snapshot is stale"},
	}, nil)
	plain := plainTranscript(m)
	for _, want := range []string{"Task plan changed", "task plan changed while this update was being prepared", "get_todo"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("transcript=%q missing %q", plain, want)
		}
	}
}
