package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
)

func TestSlashEscapePreservesComposerDraft(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.bottom.prompt().SetValue("/he")
	m.syncSlashView()
	if !m.slashOpen() {
		t.Fatal("slash view did not open")
	}

	updated, command := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(*bubbleModel)
	if command != nil {
		t.Fatalf("escape command = %v, want nil", command)
	}
	if got := m.bottom.prompt().Value(); got != "/he" {
		t.Fatalf("draft = %q, want /he", got)
	}
	if m.bottom.has(slashViewID) {
		t.Fatal("slash view remained on stack after escape")
	}
}

func TestPermissionRequestLivesInBottomPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.openPermission(permissionRequest{
		request: permission.Request{
			ToolName: "bash",
			ToolKind: permission.ToolBash,
			Detail:   "pwd",
		},
		response: make(chan permissionResponse, 1),
	})

	if top := m.bottom.top(); top == nil || top.ID() != permissionViewID {
		t.Fatalf("top view = %#v, want permission", top)
	}
	if m.bottom.composerVisible() {
		t.Fatal("composer remained visible while approval view owns input")
	}
}

func TestStalePermissionRequestAfterTurnEndIsDenied(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	response := make(chan permissionResponse, 1)
	updated, _ := model.Update(permissionRequestMsg{request: permissionRequest{
		request:  permission.Request{ToolName: "bash", ToolKind: permission.ToolBash},
		response: response,
	}})
	model = updated.(*bubbleModel)

	if model.hasPermissionView() {
		t.Fatal("stale permission request opened a modal after the turn ended")
	}
	select {
	case result := <-response:
		if result.resolution.Action != permission.ActionDeny {
			t.Fatalf("stale permission action = %s, want deny", result.resolution.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("stale permission request was not resolved")
	}
}

func TestTranscriptOverlayIncludesLiveAssistantTail(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.appendAssistantDelta("streaming now")
	m.showTranscript = true
	m.refreshTranscriptViewport(true)

	if !strings.Contains(m.transcriptOverlayView(), "streaming now") {
		t.Fatalf("transcript overlay omitted active cell: %s", m.transcriptOverlayView())
	}
	updated, _ := m.updateTranscriptKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = updated.(*bubbleModel)
	if !m.rawTranscript {
		t.Fatal("r did not toggle raw transcript mode")
	}
}

func TestInitialMessagesRestoreIntoHistoryAndNextTurn(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	messages := []model.Message{
		{Role: model.RoleUser, Content: "previous question"},
		{Role: model.RoleAssistant, Content: "previous answer"},
	}
	m := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		newPermissionBridge(),
		"",
		messages,
	)

	plain := plainTranscript(m)
	if !strings.Contains(plain, "previous question") || !strings.Contains(plain, "previous answer") {
		t.Fatalf("restored transcript = %q", plain)
	}
	if len(m.messages) != 2 {
		t.Fatalf("provider history length = %d, want 2", len(m.messages))
	}
}

func TestBashToolUsesStructuredExecCell(t *testing.T) {
	registry := newNamedTestRegistry(tool.Definition{
		Name:                "bash",
		Description:         "run shell command",
		Kind:                tool.KindBash,
		PermissionDetailKey: "command",
	})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	m := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		newPermissionBridge(),
		"",
	)
	call, err := tool.NewCall("exec-1", "bash", []byte(`{"command":"go test ./..."}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	m.appendToolCall(call)

	cell, ok := m.historyState.Active().(*ExecCell)
	if !ok {
		t.Fatalf("active cell = %T, want *ExecCell", m.historyState.Active())
	}
	if cell.Command != "go test ./..." || !cell.Running {
		t.Fatalf("exec cell = %#v", cell)
	}
}

func TestEditToolUsesStructuredPatchCell(t *testing.T) {
	registry := newNamedTestRegistry(tool.Definition{
		Name:                "apply_patch",
		Description:         "apply a workspace patch",
		Kind:                tool.KindEdit,
		PermissionDetailKey: "patch",
	})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	m := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		newPermissionBridge(),
		"",
	)
	call, err := tool.NewCall(
		"edit-1",
		"apply_patch",
		[]byte(`{"patch":"*** Begin Patch\n*** Update File: internal/a.go\n*** End Patch"}`),
	)
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	m.appendToolCall(call)

	cell, ok := m.historyState.Active().(*PatchCell)
	if !ok {
		t.Fatalf("active cell = %T, want *PatchCell", m.historyState.Active())
	}
	if len(cell.Paths) != 1 || cell.Paths[0] != "internal/a.go" {
		t.Fatalf("patch paths = %#v, want internal/a.go", cell.Paths)
	}
}
