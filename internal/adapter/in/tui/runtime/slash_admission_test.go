package runtime

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestBusyClearRejectsImmediatelyWithoutResettingConversation(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.conversation.SetMessages([]model.Message{{Role: model.RoleUser, Content: "keep me"}})
	m.busy = true
	m.panes.bottom.prompt().SetValue("/clear")

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("busy /clear command = %v, want nil", cmd)
	}
	if got := m.conversation.QueueLen(); got != 0 {
		t.Fatalf("busy /clear queue length = %d, want 0", got)
	}
	if got := m.conversation.Messages(); len(got) != 1 || got[0].Content != "keep me" {
		t.Fatalf("busy /clear mutated conversation: %#v", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "cannot clear conversation while a turn is running") {
		t.Fatalf("busy /clear rejection missing: %q", got)
	}
}

func TestBusyGoalClearRejectsWithoutChangingGoal(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.activeGoal = "finish current work"
	m.busy = true
	m.panes.bottom.prompt().SetValue("/goal clear")

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("busy /goal clear command = %v, want nil", cmd)
	}
	if m.activeGoal != "finish current work" {
		t.Fatalf("busy /goal clear changed goal to %q", m.activeGoal)
	}
	if got := m.conversation.QueueLen(); got != 0 {
		t.Fatalf("busy /goal clear queue length = %d, want 0", got)
	}
}

func TestBusyDirectCallRejectsWithoutStartingTool(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.busy = true
	beforeID := m.nextID
	m.panes.bottom.prompt().SetValue(`/call read {"path":"README.md"}`)

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("busy /call command = %v, want nil", cmd)
	}
	if m.nextID != beforeID {
		t.Fatalf("busy /call advanced tool id from %d to %d", beforeID, m.nextID)
	}
	if got := m.conversation.QueueLen(); got != 0 {
		t.Fatalf("busy /call queue length = %d, want 0", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "cannot start a direct tool call while a turn is running") {
		t.Fatalf("busy /call rejection missing: %q", got)
	}
}

func TestBusyModelMutationRejectsWithoutQueue(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.activeModel = "current-model"
	m.busy = true
	m.panes.bottom.prompt().SetValue("/model replacement-model")

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("busy /model command = %v, want nil", cmd)
	}
	if m.activeModel != "current-model" {
		t.Fatalf("busy /model changed active model to %q", m.activeModel)
	}
	if got := m.conversation.QueueLen(); got != 0 {
		t.Fatalf("busy /model queue length = %d, want 0", got)
	}
}

func TestBusyProviderListRemainsImmediatelyReadable(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAlwaysApprove, emptyTodoItems())
	m.busy = true
	m.panes.bottom.prompt().SetValue("/provider list")

	if cmd := m.submit(); cmd != nil {
		t.Fatalf("busy /provider list command = %v, want nil", cmd)
	}
	if got := m.conversation.QueueLen(); got != 0 {
		t.Fatalf("busy /provider list queue length = %d, want 0", got)
	}
	if got := plainTranscript(m); !strings.Contains(got, "No providers configured") {
		t.Fatalf("busy /provider list did not execute immediately: %q", got)
	}
}
