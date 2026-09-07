package tui

import (
	"slices"

	tododomain "github.com/projectTHORN/proton/internal/feature/todo"
)

// TodoItem is kept as a compatibility alias while TODO ownership lives in the
// domain package rather than the terminal adapter.
type TodoItem = tododomain.Item

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
		m.todoCompletionFresh = true
		m.todoCompletionDismissed = false
	} else if !isComplete {
		m.todoCompletionFresh = false
		m.todoCompletionDismissed = false
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
	if m == nil || !m.todoCompletionFresh || !allTodoCompleted(m.todo) || m.todoExpanded {
		return
	}
	m.todoCompletionFresh = false
	m.todoCompletionDismissed = true
}
