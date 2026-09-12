package runtime

import (
	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	domainmodel "github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/core/conversation"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

type scriptedRunner struct {
	events []applicationturn.Event
	result applicationturn.Result
	err    error
}

func (r *scriptedRunner) Run(ctx context.Context, _ []domainmodel.Message, sink applicationturn.Sink) (applicationturn.Result, error) {
	for _, event := range r.events {
		if err := sink(ctx, event); err != nil {
			return applicationturn.Result{}, err
		}
	}
	return r.result, r.err
}

func newTestBubbleModel(t *testing.T, mode permission.Mode, todo []tododomain.Item) *bubbleModel {
	t.Helper()
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, mode, permission.Config{})
	model := newBubbleModel(context.Background(), service, registry, todo, nil, newPermissionBridge(), "/tmp/proton")
	attachTestApplication(t, model)
	return model
}

func emptyTodoItems() []tododomain.Item {
	return []tododomain.Item{}
}

type bubbleTestHandler struct {
	definition tool.Definition
	calls      int
}

func (h *bubbleTestHandler) Definition() tool.Definition {
	return h.definition
}

func (h *bubbleTestHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls++
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: "file contents"}, nil
}

type bubbleTestRegistry struct{ handler *bubbleTestHandler }

func (r *bubbleTestRegistry) Lookup(name string) (tool.Handler, bool) {
	if name != r.handler.definition.Name {
		return nil, false
	}
	return r.handler, true
}

func (r *bubbleTestRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.definition}
}

func newBubbleTestRegistry() (*bubbleTestRegistry, *bubbleTestHandler) {
	registry := newNamedTestRegistry(tool.Definition{Name: "read", Description: "read a file", Kind: tool.KindRead, PermissionDetailKey: "path"})
	return registry, registry.handler
}

func newNamedTestRegistry(definition tool.Definition) *bubbleTestRegistry {
	handler := &bubbleTestHandler{definition: definition}
	return &bubbleTestRegistry{handler: handler}
}

func newBubbleTestService(t *testing.T, registry tool.Registry, mode permission.Mode, config permission.Config) *toolcall.Service {
	t.Helper()
	policy, err := permission.NewPolicy(config)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(mode))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestPlanModeBlocksBashBeforeAlwaysApprove(t *testing.T) {
	handler := &countingHandler{definition: tool.Definition{Name: "bash", Description: "run shell command", Kind: tool.KindBash, PermissionDetailKey: "command"}}
	registry := behaviorRegistry{handler: handler}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	model.setPlanEnabled(true)
	if !model.planMode {
		t.Fatal("plan mode was not enabled")
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
	handler := &blockingHandler{definition: tool.Definition{Name: "bash", Description: "run shell command", Kind: tool.KindBash, PermissionDetailKey: "command"}, started: make(chan struct{})}
	registry := behaviorRegistry{handler: handler}
	service := newBehaviorService(t, registry, permission.ModeAlwaysApprove)
	model := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	call, err := tool.NewCall("cancel-bash", "bash", []byte(`{"command":"sleep 30"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	command := model.startTool(call)
	messageCh := make(chan tea.Msg, 1)
	go func() {
		messageCh <- command()
	}()
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("tool did not start")
	}
	updated, cancelCommand := model.Update(testCtrl('c'))
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
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Description: "read file", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, runner, newPermissionBridge(), "")
	command := model.startTurn("wait")
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("turn did not start")
	}
	updated, cancelCommand := model.Update(testCtrl('c'))
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
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Description: "read file", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	model := newBubbleModel(context.Background(), service, registry, nil, panicRunner{}, newPermissionBridge(), "")
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
	call, err := tool.NewCall("cancel-tool", "read", []byte(`{"path":"README.md"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	model.appendToolCall(call)
	model.busy = true
	updated, _ := model.Update(turnmsg.Done{Err: context.Canceled})
	model = updated.(*bubbleModel)
	assertNoRunningTool(t, model)
	if !strings.Contains(plainTranscript(model), "cancelled") {
		t.Fatalf("cancelled tool missing terminal state: %q", plainTranscript(model))
	}
}

func TestCompletedTurnCommitsAssistantCell(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.applyTurnEvent(applicationturn.Event{Kind: applicationturn.EventTextDelta, Text: "streamed answer"})
	model.applyTurnEvent(applicationturn.Event{Kind: applicationturn.EventCompleted})
	cells := model.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("completed turn cells = %#v, want one assistant cell", cells)
	}
	got, ok := cells[0].(*AssistantCell)
	if !ok || got.Text != "streamed answer" {
		t.Fatalf("completed assistant cell = %#v", cells[0])
	}
}

func TestFailedToolReplacesRunningBlock(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.appendToolRunning("bash")
	result := tool.Result{ToolName: "bash", Failure: &tool.Failure{Code: tool.ErrorCodePermissionDenied, Message: "permission denied"}}
	model.appendToolResult(result, toolcall.ErrPermissionDenied)
	assertNoRunningTool(t, model)
	cells := model.historyState.Cells()
	if len(cells) != 1 {
		t.Fatalf("failure left duplicate cells: %#v", cells)
	}
	if _, ok := cells[0].(*ErrorCell); !ok {
		t.Fatalf("failed running cell = %#v, want error cell", cells[0])
	}
}

func TestModelToolFailureRendersReason(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	call, err := tool.NewCall("denied-tool", "read", []byte(`{"path":".env"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	model.appendToolCall(call)
	model.applyTurnEvent(applicationturn.Event{Kind: applicationturn.EventToolResult, Call: call, Result: tool.Result{CallID: call.ID, ToolName: call.Name, Denied: true, Failure: &tool.Failure{Code: tool.ErrorCodePermissionDenied, Message: "blocked by workspace policy"}}, Err: toolcall.ErrPermissionDenied})
	plain := plainTranscript(model)
	if !strings.Contains(plain, "blocked by workspace policy") {
		t.Fatalf("tool failure reason missing from transcript: %q", plain)
	}
	assertNoRunningTool(t, model)
}

func assertNoRunningTool(t *testing.T, model *bubbleModel) {
	t.Helper()
	if running := model.historyState.RunningTools(); len(running) != 0 {
		t.Fatalf("running tool remained after terminal result: %#v", running)
	}
}

type behaviorRegistry struct{ handler tool.Handler }

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

func (h *countingHandler) Definition() tool.Definition {
	return h.definition
}

func (h *countingHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls++
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: "ok"}, nil
}

type blockingHandler struct {
	definition tool.Definition
	started    chan struct{}
}

func (h *blockingHandler) Definition() tool.Definition {
	return h.definition
}

func (h *blockingHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	close(h.started)
	<-ctx.Done()
	return tool.Result{CallID: call.ID, ToolName: call.Name}, ctx.Err()
}

type blockingRunner struct{ started chan struct{} }

type panicRunner struct{}

func (panicRunner) Run(context.Context, []domainmodel.Message, applicationturn.Sink) (applicationturn.Result, error) {
	panic("test runner panic")
}

func (r *blockingRunner) Run(ctx context.Context, _ []domainmodel.Message, _ applicationturn.Sink) (applicationturn.Result, error) {
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

func TestRenderedViewportReflectsContentAndScroll(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.showWelcome = false
	m.resize(80, 12)
	for i := 0; i < 30; i++ {
		m.appendLine(fmt.Sprintf("cache-line-%02d", i))
	}
	m.refreshViewport()
	m.viewport.GotoBottom()
	bottom := ansi.Strip(m.renderedViewport())
	if !strings.Contains(bottom, "cache-line-29") {
		t.Fatalf("bottom render missing newest content: %q", bottom)
	}
	m.scrollConversationLines(-3)
	scrolled := ansi.Strip(m.renderedViewport())
	if scrolled == bottom {
		t.Fatal("scroll reused stale rendered viewport")
	}
	m.scrollConversationLines(1 << 20)
	if !m.conversationViewport.following() {
		t.Fatal("scroll to bottom did not restore follow mode")
	}
	m.appendLine("cache-new-tail")
	m.refreshViewport()
	refreshed := ansi.Strip(m.renderedViewport())
	if !strings.Contains(refreshed, "cache-new-tail") {
		t.Fatalf("content refresh reused stale rendered viewport: %q", refreshed)
	}
}

func TestSpinnerTickSkipsViewportRefreshForStreamingAssistant(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.resize(80, 24)
	m.busy = true
	m.historyState.AppendAssistantDelta("streaming assistant text")
	m.requestRelayout()
	m.reconcileLayout()
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
	m.conversationViewport.setFollowing(true)
	for i := 0; i < 40; i++ {
		m.historyState.Append(&AssistantCell{Text: fmt.Sprintf("answer %d\nmore detail", i)})
	}
	m.historyState.AppendAssistantDelta("live one\nlive two\nlive three")
	m.refreshViewport()
	if !m.conversationViewport.tailOnly {
		t.Fatal("expected streaming follow-tail viewport to use bounded tail content")
	}
	tailLines := m.viewport.TotalLineCount()
	if tailLines > m.viewport.Height() {
		t.Fatalf("tail viewport has %d lines, height %d", tailLines, m.viewport.Height())
	}
	updated, _ := m.Update(testKey(tea.KeyPgUp))
	m = updated.(*bubbleModel)
	if m.conversationViewport.tailOnly {
		t.Fatal("page up should hydrate full scrollback")
	}
	if m.viewport.TotalLineCount() <= tailLines {
		t.Fatalf("expected hydrated viewport to restore scrollback: tail=%d full=%d", tailLines, m.viewport.TotalLineCount())
	}
	if m.viewport.AtBottom() {
		t.Fatal("page up should leave the viewport above the tail")
	}
}

func TestStreamingAssistantResizeStressPreservesViewportMode(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.showWelcome = false
	m.busy = true
	m.resize(80, 24)
	for i := 0; i < 50; i++ {
		m.appendAssistant(fmt.Sprintf("history %02d with detail", i))
	}
	m.refreshViewport()
	m.conversationViewport.setFollowing(true)
	m.viewport.GotoBottom()

	sizes := [][2]int{{40, 12}, {120, 32}, {24, 8}, {60, 16}, {80, 24}}
	chunks := []string{"```go\n", "fmt.Println(\"สวัสดี 東京 👨‍💻\")\n", "// streaming chunk\n", "```\n", "final text"}
	for i, size := range sizes {
		m.appendAssistantDelta(chunks[i])
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = updated.(*bubbleModel)
		view := m.View().Content
		if !utf8.ValidString(view) {
			t.Fatalf("invalid UTF-8 after resize %dx%d", size[0], size[1])
		}
		if got := lipgloss.Width(view); got > size[0] {
			t.Fatalf("streaming frame width=%d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
		if got := lipgloss.Height(view); got > size[1] {
			t.Fatalf("streaming frame height=%d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		if !m.conversationViewport.following() {
			t.Fatalf("resize %dx%d disabled follow mode during streaming", size[0], size[1])
		}
	}

	m.scrollConversationLines(-4)
	if m.conversationViewport.following() {
		t.Fatal("scroll up did not enter reading mode")
	}
	anchor := m.captureViewportScroll()
	m.appendAssistantDelta("\nmore while reading")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 48, Height: 14})
	m = updated.(*bubbleModel)
	if m.conversationViewport.following() {
		t.Fatal("stream+resize yanked reading viewport back to follow mode")
	}
	if anchor.anchorValid {
		resolved, ok := m.historyState.ResolveScrollAnchor(anchor.anchor)
		if !ok || resolved < 0 {
			t.Fatalf("reading anchor was lost after streaming resize: resolved=%d ok=%v", resolved, ok)
		}
	}
}

func TestPlanModeAllowsProvenReadOnlyBash(t *testing.T) {
	handler := &countingHandler{definition: tool.Definition{Name: "bash", Description: "run shell command", Kind: tool.KindBash, PermissionDetailKey: "command"}}
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
	call, _ := tool.NewCall("todo-conflict", "todo", []byte(`{"action":"update","expected_revision":1,"operations":[{"op":"set_status","id":"a","status":"completed"}]}`))
	m.appendToolCall(call)
	m.applyToolResult("todo", tool.Result{CallID: "todo-conflict", ToolName: "todo", Failure: &tool.Failure{Code: tool.ErrorCodeConflict, Message: "todo snapshot is stale"}}, nil)
	plain := plainTranscript(m)
	for _, want := range []string{"Task plan changed", "task plan changed while this update was being prepared", "todo action=get"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("transcript=%q missing %q", plain, want)
		}
	}
}

func TestClearTranscriptPreservesProviderHistory(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.conversation.SetMessages([]model.Message{{Role: model.RoleUser, Content: "keep context"}})
	m.appendUser("visible message")
	m.resetTranscript()
	if len(m.conversation.Messages()) != 1 {
		t.Fatalf("clear changed provider history length = %d, want 1", len(m.conversation.Messages()))
	}
}

func TestPromptDynamicHeightAccountsForSoftWrap(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(28, 14)
	prompt := m.panes.bottom.prompt()
	prompt.SetValue(strings.Repeat("wrapped text ", 8))
	if prompt.Height() <= 1 {
		t.Fatalf("soft-wrapped prompt height = %d, want > 1", prompt.Height())
	}
	if prompt.Height() > 4 {
		t.Fatalf("soft-wrapped prompt height = %d, want <= 4", prompt.Height())
	}
	m.requestRelayout()
	m.reconcileLayout()
	if got := lipgloss.Height(m.View().Content); got > m.layout.height {
		t.Fatalf("soft-wrapped prompt frame height=%d terminal=%d", got, m.layout.height)
	}
}

func TestBracketedPasteUpdatesVisibleComposerWithoutSubmitting(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	paste := "ภาษาไทย café 東京\nsecond line\nthird line"
	updated, _ := m.Update(tea.PasteMsg{Content: paste})
	m = updated.(*bubbleModel)
	if got := m.panes.bottom.prompt().Value(); got != paste {
		t.Fatalf("pasted value = %q, want %q", got, paste)
	}
	rendered := ansi.Strip(m.promptView())
	for _, want := range []string{"ภาษาไทย", "café", "東京", "second line", "third line"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered pasted draft missing %q: %q", want, rendered)
		}
	}
	if len(m.historyState.Cells()) != 0 {
		t.Fatalf("paste submitted transcript cells: %#v", m.historyState.Cells())
	}
}

func TestLargeUnicodePasteIsNotArtificiallyCappedOrSubmitted(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	paste := strings.Repeat("ก", 25_000)
	updated, _ := m.Update(tea.PasteMsg{Content: paste})
	m = updated.(*bubbleModel)
	prompt := m.panes.bottom.prompt()
	if prompt.CharLimit != 0 {
		t.Fatalf("composer char limit = %d, want unlimited", prompt.CharLimit)
	}
	if got := prompt.Value(); got != paste {
		t.Fatalf("unicode paste runes=%d, want %d", len([]rune(got)), len([]rune(paste)))
	}
	if len(m.historyState.Cells()) != 0 || m.busy {
		t.Fatalf("large paste submitted unexpectedly: cells=%d busy=%v", len(m.historyState.Cells()), m.busy)
	}
	view := m.View().Content
	if !utf8.ValidString(view) {
		t.Fatal("large unicode paste rendered invalid UTF-8")
	}
	if got := lipgloss.Height(view); got > m.layout.height {
		t.Fatalf("large paste frame height=%d exceeds terminal=%d", got, m.layout.height)
	}
}

func TestComposerUnicodeGraphemeEditingStaysValid(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(28, 12)
	prompt := m.panes.bottom.prompt()
	prompt.SetValue("ไทย กั 👨‍💻 東京")
	prompt.CursorEnd()
	for i := 0; i < 3; i++ {
		updated, _ := m.Update(testKey(tea.KeyBackspace))
		m = updated.(*bubbleModel)
		if !utf8.ValidString(m.panes.bottom.prompt().Value()) {
			t.Fatalf("backspace produced invalid UTF-8: %q", m.panes.bottom.prompt().Value())
		}
	}
	if got := lipgloss.Height(m.View().Content); got > 12 {
		t.Fatalf("unicode edit frame height=%d exceeds terminal", got)
	}
}

func TestBashModeBracketedPasteRemainsDraft(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	updated, _ := m.Update(testText("!"))
	m = updated.(*bubbleModel)
	if !m.panes.bottom.bashMode() {
		t.Fatal("! did not enter bash mode")
	}
	paste := "printf 'ไทย 東京'\nprintf done"
	updated, _ = m.Update(tea.PasteMsg{Content: paste})
	m = updated.(*bubbleModel)
	if !m.panes.bottom.bashMode() {
		t.Fatal("paste unexpectedly left bash mode")
	}
	if got := m.panes.bottom.prompt().Value(); got != paste {
		t.Fatalf("bash pasted draft=%q want=%q", got, paste)
	}
	if len(m.historyState.Cells()) != 0 {
		t.Fatalf("bash paste submitted unexpectedly: %#v", m.historyState.Cells())
	}
}

func TestMultilinePromptUpMovesCursorInsteadOfRecallingHistory(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	prompt := m.panes.bottom.prompt()
	prompt.SetValue("first line\nsecond line")
	prompt.CursorEnd()
	updated, _ := m.Update(testKey(tea.KeyUp))
	m = updated.(*bubbleModel)
	if got := m.panes.bottom.prompt().Value(); got != "first line\nsecond line" {
		t.Fatalf("up changed multiline draft to %q", got)
	}
	if line := m.panes.bottom.prompt().Line(); line != 0 {
		t.Fatalf("up moved to logical line %d, want 0", line)
	}
}

func TestAssistantMarkdownAndWrapping(t *testing.T) {
	cell := AssistantCell{Text: "# Heading\n\n- item one\n\n```go\nfmt.Println(\"a long line that must wrap\")\n```"}
	lines := cell.RenderWidth(28)
	plain := sanitizeBubbleText(strings.Join(lines, "\n"))
	if strings.Contains(plain, "# Heading") {
		t.Fatalf("heading marker was not rendered: %q", plain)
	}
	for _, expected := range []string{"Heading", "item one", "code", "fmt.Println"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("markdown output missing %q: %q", expected, plain)
		}
	}
	for _, line := range lines {
		if width := ansi.StringWidth(line); width > 28 {
			t.Fatalf("rendered line width = %d, want <= 28: %q", width, line)
		}
	}
}

func TestSimpleANSIStyleMatchesLipglossRender(t *testing.T) {
	styles := []struct {
		name  string
		style lipgloss.Style
	}{{name: "code", style: markdownCodeStyle}, {name: "bold", style: markdownBoldStyle}, {name: "link", style: commandStyle}}
	for _, test := range styles {
		t.Run(test.name, func(t *testing.T) {
			var fast simpleANSIStyle
			var out strings.Builder
			fast.writeTo(&out, test.style, "alpha β")
			if got, want := out.String(), test.style.Render("alpha β"); got != want {
				t.Fatalf("fast style mismatch\nwant: %q\n got: %q", want, got)
			}
		})
	}
}

func TestMarkdownBodyFastPathMatchesLipglossBodyRender(t *testing.T) {
	text := "plain **bold** `code` [link](https://example.com) trailing text"
	got := strings.Join(renderMarkdownBodyWrapped(text, 24), "\n")
	want := strings.Join(renderMarkdownWrapped(text, 24, bodyStyle), "\n")
	if got != want {
		t.Fatalf("markdown body fast path mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestWrapLinesPreservesLongTokens(t *testing.T) {
	path := "/workspace/project/very-long-dangerous-command-suffix"
	wrapped := strings.Join(wrapLines(path, 12), "")
	if wrapped != path {
		t.Fatalf("long token was dropped or changed: %q", wrapped)
	}
	if got := strings.Join(strings.Fields(wrapWords("alpha beta", 5)), " "); got != "alpha beta" {
		t.Fatalf("word wrapping changed text: %q", got)
	}
}

func TestLiveViewFitsNarrowTerminal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(24, 12)
	m.appendAssistant("# Heading\n\nA very long response with a path /workspace/project/that/keeps/going")
	m.refreshViewport()
	for _, line := range strings.Split(m.View().Content, "\n") {
		if width := ansi.StringWidth(line); width > 24 {
			t.Fatalf("narrow view line width = %d, want <= 24: %q", width, line)
		}
	}
}

func TestBusyStatusUsesProfileIntentNotGenericWords(t *testing.T) {
	for _, tc := range []struct {
		profile string
		want    string
	}{
		{"universal", "Roaming"},
		{"strength", "Pushing"},
		{"intelligence", "Skilling"},
	} {
		t.Run(tc.profile, func(t *testing.T) {
			m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
			m.resize(80, 24)
			m.agentProfile = tc.profile
			m.busy = true
			m.activity = ""

			plain := ansi.Strip(m.statusView())
			if !strings.Contains(plain, tc.want) {
				t.Fatalf("status missing profile intent %q: %q", tc.want, plain)
			}
			for _, generic := range []string{"analyzing", "synthesizing", "thinking", "processing"} {
				if strings.Contains(strings.ToLower(plain), generic) {
					t.Fatalf("status leaked generic busy word %q: %q", generic, plain)
				}
			}
		})
	}
}

func TestBusyStatusPrefersRunningToolOverProfileIntent(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true
	m.activity = ""
	m.ensureHistoryState().StartToolCell(&ToolCell{CallID: "tool-1", Name: "read", Target: "internal/tui.go", Running: true})

	plain := ansi.Strip(m.statusView())
	if !strings.Contains(plain, "internal/tui.go") {
		t.Fatalf("status did not prefer the running tool: %q", plain)
	}
	if strings.Contains(plain, "Roaming") {
		t.Fatalf("status showed the profile intent while a tool was running: %q", plain)
	}
}

func TestTodoPaneEmptyStateTeachesTheSpace(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, nil)
	model.resize(80, 24)
	model.toggleTodoPane()
	model.reconcileLayout()

	view := testPlain(model.View().Content)
	if !strings.Contains(view, "No tasks in this session.") {
		t.Fatalf("todo pane missing empty-state guidance: %s", view)
	}
	if strings.Contains(view, "0/0 done") {
		t.Fatalf("todo pane showed a meaningless count when empty: %s", view)
	}
}

func TestSpinnerStopsSchedulingWhenIdle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	_, command := m.Update(spinnerTickMessage())
	if command != nil {
		t.Fatalf("idle spinner unexpectedly scheduled another tick: %v", command)
	}
}

func TestReducedMotionKeepsBusyIndicatorStatic(t *testing.T) {
	t.Setenv(envconfig.ReducedMotion, "1")
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true

	first := ansi.Strip(m.statusView())
	if !strings.Contains(first, "◌") {
		t.Fatalf("reduced motion status missing static indicator: %q", first)
	}
	for _, frame := range spinner.Dot.Frames {
		if strings.Contains(first, frame) {
			t.Fatalf("reduced motion status rendered animated frame %q: %q", frame, first)
		}
	}

	updated, _ := m.Update(spinnerTickMessage())
	m = updated.(*bubbleModel)
	second := ansi.Strip(m.statusView())
	if second != first {
		t.Fatalf("reduced motion indicator changed across ticks: %q -> %q", first, second)
	}
}

func TestReducedMotionDisablesCaretBlink(t *testing.T) {
	t.Setenv(envconfig.ReducedMotion, "1")
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	prompt := m.panes.bottom.prompt()
	if prompt.Styles().Cursor.Blink {
		t.Fatal("reduced motion left the caret blink enabled")
	}
	if _, command := m.Update(cursor.BlinkMsg{}); command != nil {
		t.Fatalf("reduced motion re-armed caret blink: %v", command)
	}
}

func TestMotionEnabledAnimatesSpinnerAndBlinksCaret(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	if m.reducedMotion {
		t.Fatal("reduced motion unexpectedly enabled by default")
	}
	if !m.panes.bottom.prompt().Styles().Cursor.Blink {
		t.Fatal("default caret blink should stay enabled")
	}

	m.busy = true
	first := ansi.Strip(m.statusView())
	updated, _ := m.Update(spinnerTickMessage())
	m = updated.(*bubbleModel)
	second := ansi.Strip(m.statusView())
	if first == second {
		t.Fatalf("busy spinner did not advance: %q", first)
	}
}

func spinnerTickMessage() tea.Msg {
	return spinner.TickMsg{}
} // spinnerTickMessage keeps this regression test independent of spinner frame
// values while still exercising the Bubble Tea message path.

func TestRefreshViewportDoesNotRenderHiddenTranscriptOverlay(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.panes.transcript.SetContent("overlay sentinel")
	m.panes.showTranscript = false
	m.appendAssistant("new visible transcript content")
	m.refreshViewport()
	if got := m.panes.transcript.View(); !strings.Contains(got, "overlay sentinel") {
		t.Fatalf("hidden transcript overlay was refreshed: %q", got)
	}
}

func TestMouseWheelOnlyScrollsInsideTranscriptViewport(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.showWelcome = false
	m.resize(80, 20)
	for i := 0; i < 60; i++ {
		m.appendLine(fmt.Sprintf("line-%02d", i))
	}
	m.refreshViewport()
	m.viewport.GotoBottom()
	m.conversationViewport.setFollowing(true)
	bottom := m.viewport.YOffset()
	updated, _ := m.Update(tea.MouseWheelMsg{X: 4, Y: m.viewport.Height() + 1, Button: tea.MouseWheelUp})
	m = updated.(*bubbleModel)
	if m.viewport.YOffset() != bottom || !m.conversationViewport.following() {
		t.Fatalf("wheel over chrome changed viewport: offset=%d want=%d follow=%v", m.viewport.YOffset(), bottom, m.conversationViewport.following())
	}
	updated, _ = m.Update(tea.MouseWheelMsg{X: 4, Y: maxInt(0, m.viewport.Height()-1), Button: tea.MouseWheelUp})
	m = updated.(*bubbleModel)
	if m.viewport.YOffset() >= bottom || m.conversationViewport.following() {
		t.Fatalf("wheel inside transcript did not scroll: offset=%d bottom=%d follow=%v", m.viewport.YOffset(), bottom, m.conversationViewport.following())
	}
}

func TestScrolledViewportDefersActiveTailRefreshUntilScroll(t *testing.T) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	m.resize(80, 18)
	m.showWelcome = false
	for i := 0; i < 40; i++ {
		m.historyState.Append(&AssistantCell{Text: fmt.Sprintf("answer %d\nmore detail", i)})
	}
	m.refreshViewport()
	m.conversationViewport.setFollowing(false)
	m.viewport.SetYOffset(maxInt(1, m.viewport.TotalLineCount()/3))
	beforeLines := m.viewport.TotalLineCount()
	beforeOffset := m.viewport.YOffset()

	m.historyState.AppendAssistantDelta("live one\nlive two\nlive three")
	m.refreshViewport()
	if !m.conversationViewport.staleTail {
		t.Fatal("expected off-screen active tail to be deferred while scrolled")
	}
	if m.viewport.TotalLineCount() != beforeLines || m.viewport.YOffset() != beforeOffset {
		t.Fatalf("deferred refresh changed viewport: lines %d->%d offset %d->%d", beforeLines, m.viewport.TotalLineCount(), beforeOffset, m.viewport.YOffset())
	}
}
func TestPageDownHydratesDeferredTail(t *testing.T) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	m.resize(80, 18)
	m.showWelcome = false
	for i := 0; i < 40; i++ {
		m.historyState.Append(&AssistantCell{Text: fmt.Sprintf("answer %d\nmore detail", i)})
	}
	m.refreshViewport()
	m.conversationViewport.setFollowing(false)
	m.viewport.SetYOffset(maxInt(1, m.viewport.TotalLineCount()/3))
	beforeLines := m.viewport.TotalLineCount()
	m.historyState.AppendAssistantDelta("live one\nlive two\nlive three")
	m.refreshViewport()

	updated, _ := m.Update(testKey(tea.KeyPgDown))
	m = updated.(*bubbleModel)
	if m.conversationViewport.staleTail || m.conversationViewport.tailOnly {
		t.Fatal("page down should hydrate deferred full scrollback")
	}
	if m.viewport.TotalLineCount() <= beforeLines {
		t.Fatalf("hydrated viewport did not include active tail: before=%d after=%d", beforeLines, m.viewport.TotalLineCount())
	}
}
func TestWelcomeCardCachesGitBranchUntilInvalidated(t *testing.T) {
	workDir := t.TempDir()
	gitDir := filepath.Join(workDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(gitDir, "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), workDir)
	m.resize(80, 24)
	first := testPlain(m.welcomeCard())
	if !strings.Contains(first, "main") {
		t.Fatalf("initial welcome branch missing: %q", first)
	}
	if err := os.WriteFile(head, []byte("ref: refs/heads/dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cached := testPlain(m.welcomeCard()); !strings.Contains(cached, "main") {
		t.Fatalf("welcome card unexpectedly reread git metadata: %q", cached)
	}
	m.invalidateWelcomeBranch()
	if refreshed := testPlain(m.welcomeCard()); !strings.Contains(refreshed, "dev") {
		t.Fatalf("invalidated welcome branch did not refresh: %q", refreshed)
	}
}

func TestScrollingRendersSingleComposer(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.syncPromptPlaceholder()
	m.showWelcome = false
	m.resize(90, 20)
	for i := 0; i < 60; i++ {
		m.appendLine(fmt.Sprintf("history-%02d", i))
	}
	m.refreshViewport()
	m.viewport.GotoBottom()
	m.conversationViewport.setFollowing(true)

	updated, _ := m.Update(testKey(tea.KeyPgUp))
	m = updated.(*bubbleModel)
	plain := ansi.Strip(m.View().Content)
	if got := strings.Count(plain, "> "); got != 1 {
		t.Fatalf("composer rendered %d times after page-up; view=%q", got, plain)
	}
	if got := lipgloss.Height(m.View().Content); got > m.layout.height {
		t.Fatalf("scrolled live view height=%d exceeds terminal height=%d", got, m.layout.height)
	}
}

func TestCommandHistoryClearsDroppedBackingSlots(t *testing.T) {
	pane := newBottomPane(false, false)
	for i := 0; i <= maxCommandHistory; i++ {
		pane.recordHistory(fmt.Sprintf("cmd-%d", i))
	}
	if got := len(pane.composer.history); got != maxCommandHistory {
		t.Fatalf("history len=%d, want %d", got, maxCommandHistory)
	}
	if got := pane.composer.history[0]; got != "cmd-1" {
		t.Fatalf("oldest retained history=%q, want cmd-1", got)
	}
	if cap(pane.composer.history) > len(pane.composer.history) {
		backing := pane.composer.history[:len(pane.composer.history)+1]
		if backing[len(pane.composer.history)] != "" {
			t.Fatalf("dropped backing slot still retains %q", backing[len(pane.composer.history)])
		}
	}
}

func TestClosingTranscriptOverlayReleasesViewportContent(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.appendAssistant("retained transcript sentinel")
	m.panes.showTranscript = true
	m.panes.rawTranscript = true
	m.refreshTranscriptViewport(true)
	if got := m.panes.transcript.View(); !strings.Contains(got, "retained transcript sentinel") {
		t.Fatalf("transcript overlay missing content before close: %q", got)
	}
	m.closeTranscriptOverlay()
	if m.panes.showTranscript {
		t.Fatal("transcript overlay remained open")
	}
	if got := m.panes.transcript.View(); strings.Contains(got, "retained transcript sentinel") {
		t.Fatalf("closed transcript overlay retained content: %q", got)
	}
}

func TestLiveConversationRetentionKeepsToolProtocolGroup(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.conversation.SetRetention(conversation.RetentionPolicy{MaxMessages: 3})
	m.conversation.SetMessages([]model.Message{
		{Role: model.RoleUser, Content: "old"},
		{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: "call-1", Name: "read", Arguments: []byte(`{"path":"README.md"}`)}}},
		{Role: model.RoleTool, ToolCallID: "call-1", ToolName: "read", Content: "result"},
		{Role: model.RoleUser, Content: "latest"},
	})
	m.conversation.RetainMessages()
	if len(m.conversation.Messages()) != 3 {
		t.Fatalf("retained message count=%d, want 3: %#v", len(m.conversation.Messages()), m.conversation.Messages())
	}
	if m.conversation.Messages()[0].Role != model.RoleAssistant || m.conversation.Messages()[1].Role != model.RoleTool || m.conversation.Messages()[2].Content != "latest" {
		t.Fatalf("live retention split protocol group: %#v", m.conversation.Messages())
	}
}
