package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

	call, err := tool.NewCall("plan-bash", "bash", []byte(`{"command":"pwd"}`))
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
