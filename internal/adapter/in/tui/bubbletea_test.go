package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	domainmodel "github.com/projectTHORN/proton/internal/adapter/out/model"
	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/engine/turn"
	tododomain "github.com/projectTHORN/proton/internal/feature/todo"
)

func TestCompletedTodoPaneIsHidden(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{
		{ID: "done", Text: "done", Status: tododomain.StatusCompleted},
		{ID: "also-done", Text: "also done", Status: tododomain.StatusCompleted},
	})
	model.resize(80, 24)
	if strings.Contains(model.View(), "TODO") {
		t.Fatalf("completed TODO pane still visible: %s", model.View())
	}
}

func TestWelcomeSitsAtTopWithoutFloatingBox(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	view := model.View()
	plain := sanitizeBubbleText(view)
	if idx := strings.Index(plain, glyphBrand); idx < 0 || idx > 8 {
		t.Fatalf("welcome is not at the top of the view: %q", plain[:minInt(80, len(plain))])
	}
	if strings.Count(view, "╭") > 1 {
		t.Fatalf("idle view has extra boxes: %s", view)
	}
}

func TestTodoPaneShowsPendingBeforeCompleted(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{
		{ID: "already-done", Text: "already done", Status: tododomain.StatusCompleted},
		{ID: "still-open", Text: "still open", Status: tododomain.StatusPending},
		{ID: "also-done", Text: "also done", Status: tododomain.StatusCompleted},
	})
	model.resize(80, 24)
	model.todoViewState.Expanded = true
	view := model.View()
	if !strings.Contains(view, "still open") {
		t.Fatalf("todo pane hid the pending item: %s", view)
	}
	pendingAt := strings.Index(view, "still open")
	doneAt := strings.Index(view, "already done")
	if doneAt >= 0 && pendingAt > doneAt {
		t.Fatal("completed todo rendered before pending todo")
	}
}

func TestPromptIsSingleRow(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	if model.prompt.Height() != 1 {
		t.Fatalf("prompt height = %d, want 1", model.prompt.Height())
	}
	if strings.Count(model.promptView(), "›") != 1 {
		t.Fatalf("prompt chrome repeated:\n%s", model.promptView())
	}
}

func TestLiveViewFitsTerminal(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{{ID: "one", Text: "one", Status: tododomain.StatusPending}})
	model.resize(80, 24)
	height := lipgloss.Height(model.View())
	if height > 24 {
		t.Fatalf("view height = %d, want <= 24:\n%s", height, model.View())
	}
}

func TestBubbleModelAcceptsTypedRunes(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	if !model.prompt.Focused() {
		t.Fatal("prompt is not focused; textarea will drop every key")
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(*bubbleModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	model = updated.(*bubbleModel)

	if got, want := model.prompt.Value(), "hi"; got != want {
		t.Fatalf("typed value = %q, want %q", got, want)
	}
}

func TestBubbleModelRendersComponentLayout(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	model := newBubbleModel(
		context.Background(),
		service,
		registry,
		[]TodoItem{{ID: "ship", Text: "ship Bubble Tea", Status: tododomain.StatusPending}},
		nil,
		newPermissionBridge(),
		"/tmp/proton",
	)
	model.resize(80, 24)
	model.appendLine("assistant: ready")
	model.refreshViewport()

	view := model.View()
	for _, expected := range []string{
		glyphBrand,
		"█▀█",
		"assistant: ready",
		"Tasks 0/1",
		"ask",
		"›",
		"/help",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("Bubble Tea view does not contain %q: %s", expected, view)
		}
	}
}

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

func TestPermissionBridgeRoundTrip(t *testing.T) {
	bridge := newPermissionBridge()
	defer bridge.Close()
	request := permission.Request{
		CallID:   "call-1",
		ToolName: "bash",
		ToolKind: permission.ToolBash,
		Detail:   "printf private",
	}
	resultCh := make(chan permission.Resolution, 1)
	errorCh := make(chan error, 1)
	go func() {
		resolution, err := bridge.Prompt(context.Background(), request)
		resultCh <- resolution
		errorCh <- err
	}()

	messageCh := make(chan tea.Msg, 1)
	go func() { messageCh <- bridge.Next()() }()
	message := <-messageCh
	pending, ok := message.(permissionRequestMsg)
	if !ok {
		t.Fatalf("bridge message = %T, want permissionRequestMsg", message)
	}
	pending.request.response <- permissionResponse{
		resolution: permission.Resolution{
			Action: permission.ActionAllow,
		},
	}
	if got := <-resultCh; got.Action != permission.ActionAllow {
		t.Fatalf("permission action = %s, want allow", got.Action)
	}
	if err := <-errorCh; err != nil {
		t.Fatalf("permission prompt error = %v, want nil", err)
	}
}

func TestBubbleModelPermissionModalRespondsToSessionGrant(t *testing.T) {
	bridge := newPermissionBridge()
	defer bridge.Close()
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	model := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		bridge,
		"",
	)
	response := make(chan permissionResponse, 1)
	model.modal = &permissionRequest{
		request: permission.Request{
			ToolName: "bash",
			ToolKind: permission.ToolBash,
			Detail:   "printf safe",
			Effect:   tool.CommandEffectReadOnly,
			Risk:     tool.CommandRiskNormal,
		},
		response: response,
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if command != nil {
		t.Fatalf("permission update command = %v, want nil", command)
	}
	model = updated.(*bubbleModel)
	if model.modal != nil {
		t.Fatal("permission modal remains open after session grant")
	}
	select {
	case result := <-response:
		if result.resolution.Action != permission.ActionAllow {
			t.Fatalf("permission action = %s, want allow", result.resolution.Action)
		}
		if result.resolution.Scope != permission.GrantScopeSession {
			t.Fatalf("permission scope = %v, want session", result.resolution.Scope)
		}
	case <-time.After(time.Second):
		t.Fatal("permission modal did not send a response")
	}
}

func TestBubbleModelHistoryUsesTextarea(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
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
	model.prompt.SetValue(":help")
	if command := model.submit(); command != nil {
		t.Fatal("help submit command != nil")
	}
	model.historyPrevious()
	if got, want := model.prompt.Value(), ":help"; got != want {
		t.Fatalf("history value = %q, want %q", got, want)
	}
	model.historyNext()
	if got := model.prompt.Value(); got != "" {
		t.Fatalf("history next value = %q, want empty", got)
	}
}

func TestSanitizeBubbleTextRemovesControlCharacters(t *testing.T) {
	got := sanitizeBubbleText("hello\x1b[31m\nworld")
	if strings.ContainsRune(got, '\x1b') {
		t.Fatalf("sanitized text contains escape: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("sanitized text contains newline: %q", got)
	}
}

func TestEmptyStateWithoutRunnerGuidesSlashCommands(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)

	view := model.View()
	for _, expected := range []string{
		"Type a message or /command",
		glyphBrand,
		"█▀█",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("empty state view does not contain %q: %s", expected, view)
		}
	}
	if got, want := model.prompt.Placeholder, "Type a message or /command…"; got != want {
		t.Fatalf("placeholder = %q, want %q", got, want)
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

func TestPermissionCardOverlaysTranscript(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.appendLine("assistant: ready")
	model.refreshViewport()
	model.modal = &permissionRequest{
		request: permission.Request{
			ToolName:  "bash",
			ToolKind:  permission.ToolBash,
			Detail:    "rm -rf tmp",
			Arguments: json.RawMessage(`{"command":"rm -rf tmp"}`),
		},
		response: make(chan permissionResponse, 1),
	}

	if !strings.Contains(plainTranscript(model), "assistant: ready") {
		t.Fatalf("overlay replaced the transcript: %#v", model.blocks)
	}
	view := model.View()
	for _, expected := range []string{
		"Permission required — shell modifies state",
		"bash",
		"Allow once",
		"Deny",
		"esc review",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("overlay view does not contain %q: %s", expected, view)
		}
	}
	if strings.Contains(view, "Allow for this request this session") || strings.Contains(view, "s session") {
		t.Fatalf("mutating request exposed session grant: %s", view)
	}
}

func TestPermissionEscParksForScroll(t *testing.T) {
	response := make(chan permissionResponse, 1)
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.modal = &permissionRequest{
		request: permission.Request{
			ToolName: "read_file",
			ToolKind: permission.ToolRead,
			Detail:   "README.md",
		},
		response: response,
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if command != nil {
		t.Fatalf("esc review command = %v, want nil", command)
	}
	model = updated.(*bubbleModel)
	if model.modal == nil {
		t.Fatal("esc dismissed the permission card")
	}
	if !model.modalParked {
		t.Fatal("esc did not enter transcript review mode")
	}
	if !strings.Contains(model.View(), "tab review") {
		t.Fatalf("review view missing approval hint: %s", model.View())
	}

	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	if command != nil {
		t.Fatalf("tab command = %v, want nil", command)
	}
	model = updated.(*bubbleModel)
	if model.modalParked {
		t.Fatal("tab did not return focus to the permission card")
	}

	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if command != nil {
		t.Fatalf("deny command = %v, want nil", command)
	}
	model = updated.(*bubbleModel)
	if model.modal != nil {
		t.Fatal("deny left the permission card open")
	}
	select {
	case result := <-response:
		if result.resolution.Action != permission.ActionDeny {
			t.Fatalf("permission action = %s, want deny", result.resolution.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("deny did not send a response")
	}
}

func TestPermissionCtrlCCancelsTurnInsteadOfDenying(t *testing.T) {
	response := make(chan permissionResponse, 1)
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.busy = true
	canceled := false
	model.turnCancel = func() { canceled = true }
	model.modal = &permissionRequest{
		request:  permission.Request{ToolName: "bash", ToolKind: permission.ToolBash, Detail: "pwd", Arguments: json.RawMessage(`{"command":"pwd"}`)},
		response: response,
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(*bubbleModel)
	if command != nil {
		t.Fatalf("ctrl+c command = %v, want nil while canceling active turn", command)
	}
	if !canceled {
		t.Fatal("ctrl+c did not cancel active turn")
	}
	select {
	case got := <-response:
		t.Fatalf("ctrl+c resolved permission unexpectedly: %+v", got)
	default:
	}
}

func TestPermissionBashRiskPresentationUsesCommandEffect(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	tests := []struct {
		name    string
		command string
		want    string
	}{
		{name: "read only", command: "pwd", want: "Permission request — shell read only"},
		{name: "mutating", command: "rm -rf tmp", want: "Permission required — shell modifies state"},
		{name: "unknown", command: "make test", want: "Permission required — shell effects unknown"},
		{name: "composed mutation", command: "pwd && rm tmp", want: "Permission required — shell modifies state"},
		{name: "remote", command: "git push origin main", want: "Permission required — modifies remote"},
		{name: "publish", command: "npm publish", want: "Permission required — publishes package"},
		{name: "deployment", command: "wrangler deploy", want: "Permission required — changes deployment"},
		{name: "destructive deployment", command: "terraform destroy", want: "Permission required — destructive deployment change"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model.bottom.remove(permissionViewID)
			model.modal = &permissionRequest{
				request: permission.Request{
					ToolName:  "bash",
					ToolKind:  permission.ToolBash,
					Detail:    tc.command,
					Arguments: json.RawMessage(fmt.Sprintf(`{"command":%q}`, tc.command)),
				},
				response: make(chan permissionResponse, 1),
			}
			view := model.View()
			if !strings.Contains(view, tc.want) {
				t.Fatalf("view missing %q:\n%s", tc.want, view)
			}
		})
	}
}

func TestPermissionOptionListEnterAndNumbers(t *testing.T) {
	response := make(chan permissionResponse, 1)
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.modal = &permissionRequest{
		request: permission.Request{
			ToolName: "bash",
			ToolKind: permission.ToolBash,
			Detail:   "ls",
			Effect:   tool.CommandEffectReadOnly,
			Risk:     tool.CommandRiskNormal,
		},
		response: response,
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(*bubbleModel)
	if model.permIndex != 1 {
		t.Fatalf("permIndex after down = %d, want 1", model.permIndex)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(*bubbleModel)
	if model.modal != nil {
		t.Fatal("enter left permission modal open")
	}
	select {
	case result := <-response:
		if result.resolution.Scope != permission.GrantScopeSession {
			t.Fatalf("enter on session row scope = %v", result.resolution.Scope)
		}
	case <-time.After(time.Second):
		t.Fatal("enter did not resolve permission")
	}

	response = make(chan permissionResponse, 1)
	model.modal = &permissionRequest{
		request: permission.Request{
			ToolName: "bash", ToolKind: permission.ToolBash,
			Effect: tool.CommandEffectReadOnly, Risk: tool.CommandRiskNormal,
		},
		response: response,
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	model = updated.(*bubbleModel)
	select {
	case result := <-response:
		if result.resolution.Action != permission.ActionDeny {
			t.Fatalf("number 3 action = %s, want deny", result.resolution.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("number shortcut did not resolve")
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

func TestRefreshViewportPreservesScrollWhenNotFollowing(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	for range 40 {
		model.appendLine("line")
	}
	model.refreshViewport()
	model.viewport.GotoTop()
	model.followTail = false

	model.appendLine("tail")
	model.refreshViewport()
	if model.viewport.AtBottom() {
		t.Fatal("refreshViewport followed the tail after the user scrolled up")
	}
	if model.followTail {
		t.Fatal("followTail was re-enabled after a mid-scroll append")
	}
}

func TestSlashDropdownFiltersAndTabAccepts(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.prompt.SetValue("/he")
	if !model.slashOpen() {
		t.Fatal("slash dropdown did not open for /he")
	}
	matches := model.slashMatches()
	if len(matches) != 1 || matches[0].name != "help" {
		t.Fatalf("slash matches = %#v, want help", matches)
	}
	applied, command := model.acceptSlash(false)
	if !applied || command != nil {
		t.Fatalf("tab accept applied=%v command=%v", applied, command)
	}
	if got := model.prompt.Value(); got != "/help" {
		t.Fatalf("tab accept value = %q, want /help", got)
	}
}

func TestColonAliasDispatchesHelp(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.prompt.SetValue(":help")
	if command := model.submit(); command != nil {
		t.Fatalf("colon help command = %v, want nil", command)
	}
	if !strings.Contains(plainTranscript(model), "/call") {
		t.Fatalf("colon alias did not render help: %q", plainTranscript(model))
	}
}

func TestShiftTabCyclesAskPlanAlwaysApprove(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(*bubbleModel)
	if !model.planMode {
		t.Fatal("first shift+tab did not enter plan")
	}
	if model.service.Mode() != permission.ModeAsk {
		t.Fatalf("plan cycle changed mode = %s", model.service.Mode())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(*bubbleModel)
	if model.planMode {
		t.Fatal("second shift+tab left plan on")
	}
	if model.service.Mode() != permission.ModeAlwaysApprove {
		t.Fatalf("second shift+tab mode = %s, want always-approve", model.service.Mode())
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(*bubbleModel)
	if model.planMode || model.service.Mode() != permission.ModeAsk {
		t.Fatalf("third shift+tab = plan=%v mode=%s", model.planMode, model.service.Mode())
	}
}

func TestWelcomeCardReprintsAfterClear(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(80, 24)
	model.appendLine("gone")
	model.prompt.SetValue("/clear")
	_ = model.submit()
	model.refreshViewport()
	view := model.View()
	if strings.Contains(plainTranscript(model), "gone") {
		t.Fatal("clear left transcript body")
	}
	if !strings.Contains(view, glyphBrand) || !strings.Contains(view, "█▀█") {
		t.Fatalf("clear did not reprint welcome: %s", view)
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
	command := model.startTurn("hi")
	if command == nil {
		t.Fatal("startTurn command = nil")
	}
	for range 3 {
		message := command()
		updated, next := model.Update(message)
		model = updated.(*bubbleModel)
		command = next
		if command == nil {
			break
		}
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

type scriptedRunner struct {
	events []applicationturn.Event
	result applicationturn.Result
	err    error
}

func (r *scriptedRunner) Run(
	ctx context.Context,
	_ []domainmodel.Message,
	sink applicationturn.Sink,
) (applicationturn.Result, error) {
	for _, event := range r.events {
		if err := sink(ctx, event); err != nil {
			return applicationturn.Result{}, err
		}
	}
	return r.result, r.err
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

func newTestBubbleModel(
	t *testing.T,
	mode permission.Mode,
	todo []TodoItem,
) *bubbleModel {
	t.Helper()
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, mode, permission.Config{})
	return newBubbleModel(
		context.Background(),
		service,
		registry,
		todo,
		nil,
		newPermissionBridge(),
		"/tmp/proton",
	)
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
	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   "file contents",
	}, nil
}

type bubbleTestRegistry struct {
	handler *bubbleTestHandler
}

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
	registry := newNamedTestRegistry(tool.Definition{
		Name:                "read_file",
		Description:         "read a file",
		Kind:                tool.KindRead,
		PermissionDetailKey: "path",
	})
	return registry, registry.handler
}

func newNamedTestRegistry(definition tool.Definition) *bubbleTestRegistry {
	handler := &bubbleTestHandler{definition: definition}
	return &bubbleTestRegistry{handler: handler}
}

func newBubbleTestService(
	t *testing.T,
	registry tool.Registry,
	mode permission.Mode,
	config permission.Config,
) *toolcall.Service {
	t.Helper()
	policy, err := permission.NewPolicy(config)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(
		registry,
		policy,
		toolcall.WithMode(mode),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func TestSpinnerLifecycle(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, nil)

	t.Run("tick returns single tick command without double-batching", func(t *testing.T) {
		model.busy = true
		_, cmd := model.Update(spinner.TickMsg{})
		if cmd == nil {
			t.Fatal("expected non-nil cmd, got nil")
		}
		msg := cmd()
		if _, isBatch := msg.(tea.BatchMsg); isBatch {
			t.Fatal("cmd returned BatchMsg, indicating exponential double-batching of ticks")
		}
	})
}

func TestFormatElapsed(t *testing.T) {
	cases := []struct {
		duration time.Duration
		want     string
	}{
		{0, "0s"},
		{500 * time.Millisecond, "0s"},
		{999 * time.Millisecond, "0s"},
		{time.Second, "1s"},
		{5 * time.Second, "5s"},
		{60 * time.Second, "1m0s"},
		{61 * time.Second, "1m1s"},
	}
	for _, tc := range cases {
		got := formatElapsed(tc.duration)
		if got != tc.want {
			t.Errorf("formatElapsed(%v) = %q, want %q", tc.duration, got, tc.want)
		}
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

func TestPermissionBashPresentationShowsCwdAndEffectReason(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	model.resize(100, 30)
	model.modal = &permissionRequest{
		request: permission.Request{
			ToolName: "bash", ToolKind: permission.ToolBash, Detail: "git status --short",
			Arguments: json.RawMessage(`{"command":"git status --short","cwd":"internal/agent"}`),
		},
		response: make(chan permissionResponse, 1),
	}
	view := model.View()
	for _, want := range []string{"Cwd: internal/agent", "Effect: read_only", "git status is read only"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}
