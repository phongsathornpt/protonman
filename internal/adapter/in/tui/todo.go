package tui

import (
	"slices"

	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

// TodoItem is kept as a compatibility alias while TODO ownership lives in the
// domain package rather than the terminal adapter.
type TodoItem = tododomain.Item

type todoViewState struct {
	Expanded    bool
	ShowRetired bool
}

type todoLifecycleState struct {
	CompletionFresh     bool
	CompletionDismissed bool
}

func (m *bubbleModel) syncTodoSnapshot() bool {
	if m == nil || m.todoStore == nil {
		return false
	}
	snapshot := m.todoStore.Snapshot()
	if snapshot.Revision == m.todoRevision && slices.Equal(snapshot.Items, m.todo) {
		return false
	}
	wasComplete := allTodoCompleted(m.todo)
	m.todo = tododomain.CloneItems(snapshot.Items)
	m.todoRevision = snapshot.Revision
	isComplete := allTodoCompleted(m.todo)
	if isComplete && !wasComplete {
		m.todoLifecycle.CompletionFresh = true
		m.todoLifecycle.CompletionDismissed = false
	} else if !isComplete {
		m.todoLifecycle.CompletionFresh = false
		m.todoLifecycle.CompletionDismissed = false
		m.todoViewState.ShowRetired = false
	}
	return true
}

func allTodoCompleted(items []TodoItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item.Status != tododomain.StatusCompleted {
			return false
		}
	}
	return true
}

func (m *bubbleModel) retireCompletedTodoForNextTurn() {
	if m == nil || !m.todoLifecycle.CompletionFresh || !allTodoCompleted(m.todo) {
		return
	}
	m.todoLifecycle.CompletionFresh = false
	m.todoLifecycle.CompletionDismissed = true
	m.todoViewState.ShowRetired = false
}

func (m *bubbleModel) revealRetiredTodo() {
	if m != nil && m.todoLifecycle.CompletionDismissed {
		m.todoViewState.ShowRetired = true
	}
}
