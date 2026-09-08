package runtime

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	"slices"
)

type TodoItem = tododomain.Item // TodoItem is kept as a compatibility alias while TODO ownership lives in the
// domain package rather than the terminal adapter.

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

const todoInspectViewID = "todo-inspect"

type todoPaneView struct{ offset int }

func (*todoPaneView) ID() string {
	return todoInspectViewID
}

func (*todoPaneView) ReplacesComposer() bool {
	return false
}

func (v *todoPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	limit := todoInspectionLimit(m.height)
	maxOffset := maxInt(0, len(m.todo)-limit)
	switch message.String() {
	case "esc", "enter":
		m.bottom.remove(todoInspectViewID)
		return true, nil
	case "up", "k":
		v.offset = maxInt(0, v.offset-1)
		return true, nil
	case "down", "j":
		v.offset = minInt(maxOffset, v.offset+1)
		return true, nil
	default:
		return false, nil
	}
}

func (v *todoPaneView) Render(m *bubbleModel) string {
	rows := pane.TodoRows(pane.TodoSnapshot{Width: m.width, Height: m.height, Offset: v.offset, Items: m.todo})
	return renderModalRows(m, promptBorder, rows)
}

func todoInspectionLimit(height int) int {
	return pane.TodoInspectionLimit(height)
}

func (m *bubbleModel) toggleTodoPane() {
	if m.bottom.has(todoInspectViewID) {
		m.bottom.remove(todoInspectViewID)
	} else {
		m.bottom.push(&todoPaneView{})
	}
	m.relayout()
}
