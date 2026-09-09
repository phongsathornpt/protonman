package runtime

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	todopane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/todo"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	"slices"
)

type TodoItem = tododomain.Item // TodoItem is kept as a compatibility alias while TODO ownership lives in the
// domain package rather than the terminal adapter.

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
}

const todoInspectViewID = "todo-inspect"

type todoListItem struct {
	item TodoItem
}

func (i todoListItem) FilterValue() string { return i.item.Text + " " + i.item.ID }
func (i todoListItem) Description() string { return "id: " + strings.TrimSpace(i.item.ID) }
func (i todoListItem) Title() string {
	glyph := "○ "
	switch i.item.Status {
	case tododomain.StatusInProgress:
		glyph = "● "
	case tododomain.StatusCompleted:
		glyph = "✓ "
	}
	return glyph + i.item.Text
}

type todoPaneView struct {
	picker      list.Model
	initialized bool
}

func (*todoPaneView) ID() string             { return todoInspectViewID }
func (*todoPaneView) ReplacesComposer() bool { return false }

func (v *todoPaneView) ensurePicker(m *bubbleModel) {
	if v.initialized || m == nil {
		return
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = true
	v.picker = list.New(todoListItems(m.todo), delegate, maxInt(12, m.width-8), maxInt(5, minInt(14, m.height-4)))
	v.picker.DisableQuitKeybindings()
	v.picker.SetFilteringEnabled(false)
	v.picker.SetStatusBarItemName("task", "tasks")
	v.initialized = true
	v.syncTitle(m)
}

func todoListItems(items []TodoItem) []list.Item {
	ordered := tododomain.CloneItems(items)
	slices.SortStableFunc(ordered, func(a, b TodoItem) int {
		return todoStatusPriority(a.Status) - todoStatusPriority(b.Status)
	})
	out := make([]list.Item, 0, len(ordered))
	for _, item := range ordered {
		out = append(out, todoListItem{item: item})
	}
	return out
}

func todoStatusPriority(status tododomain.Status) int {
	switch status {
	case tododomain.StatusInProgress:
		return 0
	case tododomain.StatusPending:
		return 1
	default:
		return 2
	}
}

func (v *todoPaneView) syncTitle(m *bubbleModel) {
	if !v.initialized || m == nil {
		return
	}
	completed, _, _ := todopane.TodoCounts(m.todo)
	v.picker.Title = fmt.Sprintf("Tasks · %d/%d done", completed, len(m.todo))
}

func (v *todoPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	v.ensurePicker(m)
	switch message.String() {
	case "esc", "enter":
		m.bottom.remove(todoInspectViewID)
		return true, nil
	}
	updated, cmd := v.picker.Update(message)
	v.picker = updated
	return true, cmd
}

func (v *todoPaneView) Render(m *bubbleModel) string {
	v.ensurePicker(m)
	if !v.initialized {
		return ""
	}
	v.picker.SetItems(todoListItems(m.todo))
	v.syncTitle(m)
	v.picker.SetSize(maxInt(12, m.width-8), maxInt(4, minInt(8, m.height-6)))
	v.picker.SetShowStatusBar(false)
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(false)
	return renderModalRows(m, promptBorder, strings.Split(v.picker.View(), "\n"))
}

func (m *bubbleModel) toggleTodoPane() {
	if m.bottom.has(todoInspectViewID) {
		m.bottom.remove(todoInspectViewID)
	} else {
		m.bottom.push(&todoPaneView{})
	}
	m.relayout()
}
