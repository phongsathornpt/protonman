package runtime

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestSlashCommandBypassesQueueWhileBusy(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.busy = true
	m.panes.bottom.prompt().SetValue("/help")

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("busy /help command = %v, want nil", cmd)
	}
	if got := m.conversation.QueueLen(); got != 0 {
		t.Fatalf("busy /help queue length = %d, want 0", got)
	}
	if got := m.panes.bottom.prompt().Value(); got != "" {
		t.Fatalf("busy /help prompt = %q, want empty", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "/help") || !strings.Contains(got, "/quit") {
		t.Fatalf("busy /help did not execute immediately: %q", got)
	}
}

func TestUnknownSlashCommandBypassesQueueWhileBusy(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.busy = true
	m.panes.bottom.prompt().SetValue("/definitely-missing")

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("busy unknown slash command = %v, want nil", cmd)
	}
	if got := m.conversation.QueueLen(); got != 0 {
		t.Fatalf("busy unknown slash queue length = %d, want 0", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, `unknown command "definitely-missing"`) {
		t.Fatalf("busy unknown slash was not rejected immediately: %q", got)
	}
}

func TestBusySlashMutationRejectsImmediatelyWithoutQueue(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.busy = true
	m.panes.bottom.prompt().SetValue("/goal replace active work")

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("busy /goal command = %v, want nil", cmd)
	}
	if got := m.conversation.QueueLen(); got != 0 {
		t.Fatalf("busy /goal queue length = %d, want 0", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "cannot change goal while a turn is running") {
		t.Fatalf("busy /goal did not reject immediately: %q", got)
	}
}

func TestSlashCommandBypassesQueueWithPermissionView(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.openPermission(permissionRequest{
		Request: permission.Request{
			ToolName:  "bash",
			ToolKind:  permission.ToolBash,
			Detail:    "echo hi",
			Arguments: json.RawMessage(`{"command":"echo hi"}`),
		},
		Response: make(chan permissionResponse, 1),
	})
	m.panes.bottom.prompt().SetValue("/help")

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("permission /help command = %v, want nil", cmd)
	}
	if got := m.conversation.QueueLen(); got != 0 {
		t.Fatalf("permission /help queue length = %d, want 0", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "/help") {
		t.Fatalf("permission /help did not execute immediately: %q", got)
	}
}

func TestNormalPromptStillQueuesWhileBusy(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.busy = true
	m.panes.bottom.prompt().SetValue("continue implementation")

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("busy prompt command = %v, want nil", cmd)
	}
	if got := m.conversation.QueueLen(); got != 1 {
		t.Fatalf("busy prompt queue length = %d, want 1", got)
	}
	if got := m.conversation.Queue()[0]; got != "continue implementation" {
		t.Fatalf("queued prompt = %q, want %q", got, "continue implementation")
	}
}
