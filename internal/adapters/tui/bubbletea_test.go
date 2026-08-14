package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

func TestBubbleModelRendersComponentLayout(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	model := newBubbleModel(
		context.Background(),
		service,
		registry,
		[]TodoItem{{Text: "ship Bubble Tea", Done: false}},
		nil,
		newPermissionBridge(),
	)
	model.resize(80, 24)
	model.appendLine("assistant: ready")
	model.refreshViewport()

	view := model.View()
	for _, expected := range []string{
		"PROTON",
		"assistant: ready",
		"TODO 0/1 complete",
		"ship Bubble Tea",
		"permission: ask",
		"❯",
		"ctrl+l",
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
	model = updated.(bubbleModel)
	if handler.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", handler.calls)
	}
	if !strings.Contains(strings.Join(model.scrollback, "\n"), "file contents") {
		t.Fatalf("scrollback does not contain tool output: %#v", model.scrollback)
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
	)
	response := make(chan permissionResponse, 1)
	model.modal = &permissionRequest{
		request: permission.Request{
			ToolName: "bash",
			ToolKind: permission.ToolBash,
			Detail:   "printf safe",
		},
		response: response,
	}

	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if command != nil {
		t.Fatalf("permission update command = %v, want nil", command)
	}
	model = updated.(bubbleModel)
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
	handler := &bubbleTestHandler{
		definition: tool.Definition{
			Name:                "read_file",
			Description:         "read a file",
			Kind:                tool.KindRead,
			PermissionDetailKey: "path",
		},
	}
	return &bubbleTestRegistry{handler: handler}, handler
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
