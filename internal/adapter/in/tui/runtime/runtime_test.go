package runtime

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	domainmodel "github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/adapter/out/sessionfs"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func newTestBubbleModel(t *testing.T, mode permission.Mode, todo []TodoItem) *bubbleModel {
	t.Helper()
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, mode, permission.Config{})
	return newBubbleModel(context.Background(), service, registry, todo, nil, newPermissionBridge(), "/tmp/proton")
}

func emptyTodoItems() []TodoItem {
	return []TodoItem{}
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
	registry := newNamedTestRegistry(tool.Definition{Name: "read_file", Description: "read a file", Kind: tool.KindRead, PermissionDetailKey: "path"})
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
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read_file", Description: "read file", Kind: tool.KindRead}}}
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
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read_file", Description: "read file", Kind: tool.KindRead}}}
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
	result := tool.Result{ToolName: "bash", Failure: &tool.Failure{Code: tool.ErrorCodePermissionDenied, Message: "permission denied"}}
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
	model.applyTurnEvent(applicationturn.Event{Kind: applicationturn.EventToolResult, Call: call, Result: tool.Result{CallID: call.ID, ToolName: call.Name, Denied: true, Failure: &tool.Failure{Code: tool.ErrorCodePermissionDenied, Message: "blocked by workspace policy"}}, Err: toolcall.ErrPermissionDenied})
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
	call, _ := tool.NewCall("todo-conflict", "update_todo", []byte(`{"expected_revision":1,"operations":[{"op":"set_status","id":"a","status":"completed"}]}`))
	m.appendToolCall(call)
	m.applyToolResult("update_todo", tool.Result{CallID: "todo-conflict", ToolName: "update_todo", Failure: &tool.Failure{Code: tool.ErrorCodeConflict, Message: "todo snapshot is stale"}}, nil)
	plain := plainTranscript(m)
	for _, want := range []string{"Task plan changed", "task plan changed while this update was being prepared", "todo action=get"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("transcript=%q missing %q", plain, want)
		}
	}
}

func TestNewConversationClearsProviderHistory(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.messages = []model.Message{{Role: model.RoleUser, Content: "old context"}}
	m.appendUser("visible old context")
	m.queue = []string{"queued"}
	if command := m.executeCommand("/new"); command != nil {
		t.Fatalf("/new command = %v, want nil", command)
	}
	if len(m.messages) != 0 {
		t.Fatalf("provider history length = %d, want 0", len(m.messages))
	}
	if len(m.queue) != 0 || len(m.historyState.Cells()) != 0 {
		t.Fatalf("new conversation retained state: queue=%v cells=%v", m.queue, m.historyState.Cells())
	}
}

func TestClearTranscriptPreservesProviderHistory(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.messages = []model.Message{{Role: model.RoleUser, Content: "keep context"}}
	m.appendUser("visible message")
	m.resetTranscript()
	if len(m.messages) != 1 {
		t.Fatalf("clear changed provider history length = %d, want 1", len(m.messages))
	}
}

func TestMultilinePromptUpMovesCursorInsteadOfRecallingHistory(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	prompt := m.bottom.prompt()
	prompt.SetValue("first line\nsecond line")
	prompt.CursorEnd()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(*bubbleModel)
	if got := m.bottom.prompt().Value(); got != "first line\nsecond line" {
		t.Fatalf("up changed multiline draft to %q", got)
	}
	if line := m.bottom.prompt().Line(); line != 0 {
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
	for _, line := range strings.Split(m.View(), "\n") {
		if width := ansi.StringWidth(line); width > 24 {
			t.Fatalf("narrow view line width = %d, want <= 24: %q", width, line)
		}
	}
}

func TestSpinnerStopsSchedulingWhenIdle(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	_, command := m.Update(spinnerTickMessage())
	if command != nil {
		t.Fatalf("idle spinner unexpectedly scheduled another tick: %v", command)
	}
}

func spinnerTickMessage() tea.Msg {
	return spinner.TickMsg{}
} // spinnerTickMessage keeps this regression test independent of spinner frame
// values while still exercising the Bubble Tea message path.

func TestRefreshViewportDoesNotRenderHiddenTranscriptOverlay(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.transcriptViewport.SetContent("overlay sentinel")
	m.showTranscript = false
	m.appendAssistant("new visible transcript content")
	m.refreshViewport()
	if got := m.transcriptViewport.View(); !strings.Contains(got, "overlay sentinel") {
		t.Fatalf("hidden transcript overlay was refreshed: %q", got)
	}
}

func TestSessionCommandsExposeIdentityAndWorkspaceSessions(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.sessionID = "current-session"
	m.workspaceKey = "workspace-key"
	m.messages = []model.Message{{Role: model.RoleUser, Content: "hello"}}
	m.executeCommand("/session")
	content := m.historyState.RenderContent()
	for _, want := range []string{"current-session", "workspace-key", "messages: 1"} {
		if !strings.Contains(content, want) {
			t.Fatalf("/session missing %q: %s", want, content)
		}
	}
	store, err := sessionfs.NewFileStore(filepath.Join(t.TempDir(), "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), "current-session", session.State{PermissionMode: permission.ModeAsk.String(), WorkspaceKey: "workspace-key", AgentProfile: "dex", Messages: []session.Message{{Role: model.RoleUser, Content: "resume this work"}}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), "other-session", session.State{PermissionMode: permission.ModeAsk.String(), WorkspaceKey: "other", Messages: []session.Message{{Role: model.RoleUser, Content: "do not show"}}}); err != nil {
		t.Fatal(err)
	}
	m.sessions = app.NewSessions(store)
	m.executeCommand("/sessions")
	content = m.historyState.RenderContent()
	for _, want := range []string{"Recent sessions:", "current-session", "resume this work", "protonman session resume"} {
		if !strings.Contains(content, want) {
			t.Fatalf("/sessions missing %q: %s", want, content)
		}
	}
	if strings.Contains(content, "other-session") {
		t.Fatalf("cross-workspace session leaked: %s", content)
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
	m.followTail = true
	bottom := m.viewport.YOffset
	updated, _ := m.Update(tea.MouseMsg{X: 4, Y: m.viewport.Height + 1, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	m = updated.(*bubbleModel)
	if m.viewport.YOffset != bottom || !m.followTail {
		t.Fatalf("wheel over chrome changed viewport: offset=%d want=%d follow=%v", m.viewport.YOffset, bottom, m.followTail)
	}
	updated, _ = m.Update(tea.MouseMsg{X: 4, Y: maxInt(0, m.viewport.Height-1), Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	m = updated.(*bubbleModel)
	if m.viewport.YOffset >= bottom || m.followTail {
		t.Fatalf("wheel inside transcript did not scroll: offset=%d bottom=%d follow=%v", m.viewport.YOffset, bottom, m.followTail)
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
	m.followTail = false
	m.viewport.SetYOffset(maxInt(1, m.viewport.TotalLineCount()/3))
	beforeLines := m.viewport.TotalLineCount()
	beforeOffset := m.viewport.YOffset

	m.historyState.AppendAssistantDelta("live one\nlive two\nlive three")
	m.refreshViewport()
	if !m.viewportStaleTail {
		t.Fatal("expected off-screen active tail to be deferred while scrolled")
	}
	if m.viewport.TotalLineCount() != beforeLines || m.viewport.YOffset != beforeOffset {
		t.Fatalf("deferred refresh changed viewport: lines %d->%d offset %d->%d", beforeLines, m.viewport.TotalLineCount(), beforeOffset, m.viewport.YOffset)
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
	m.followTail = false
	m.viewport.SetYOffset(maxInt(1, m.viewport.TotalLineCount()/3))
	beforeLines := m.viewport.TotalLineCount()
	m.historyState.AppendAssistantDelta("live one\nlive two\nlive three")
	m.refreshViewport()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(*bubbleModel)
	if m.viewportStaleTail || m.viewportTailOnly {
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
	first := m.welcomeCard()
	if !strings.Contains(first, "git:(main)") {
		t.Fatalf("initial welcome branch missing: %q", first)
	}
	if err := os.WriteFile(head, []byte("ref: refs/heads/dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cached := m.welcomeCard(); !strings.Contains(cached, "git:(main)") {
		t.Fatalf("welcome card unexpectedly reread git metadata: %q", cached)
	}
	m.invalidateWelcomeBranch()
	if refreshed := m.welcomeCard(); !strings.Contains(refreshed, "git:(dev)") {
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
	m.followTail = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = updated.(*bubbleModel)
	plain := ansi.Strip(m.View())
	placeholder := "Ask Protonman to inspect or change this workspace"
	if got := strings.Count(plain, placeholder); got != 1 {
		t.Fatalf("composer rendered %d times after page-up; view=%q", got, plain)
	}
	if got := lipgloss.Height(m.View()); got > m.height {
		t.Fatalf("scrolled live view height=%d exceeds terminal height=%d", got, m.height)
	}
}
