package tui

import tododomain "github.com/projectTHORN/proton/internal/todo"

// TodoItem is kept as a compatibility alias while TODO ownership lives in the
// domain package rather than the terminal adapter.
type TodoItem = tododomain.Item

// ParseTODO preserves the existing TUI-facing parser entry point while using
// the provider-neutral TODO domain parser.
func ParseTODO(markdown string) []TodoItem {
	return tododomain.ParseMarkdown(markdown)
}

func (m *bubbleModel) syncTodoSnapshot() bool {
	if m == nil || m.todoStore == nil {
		return false
	}
	snapshot := m.todoStore.Snapshot()
	if snapshot.Revision == m.todoRevision {
		return false
	}
	m.todo = tododomain.CloneItems(snapshot.Items)
	m.todoRevision = snapshot.Revision
	return true
}
