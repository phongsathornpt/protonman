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

func allTodoCompleted(items []tododomain.Item) bool {
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
	item tododomain.Item
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

func (*todoPaneView) ID() string                             { return todoInspectViewID }
func (*todoPaneView) PresentationMode() panePresentationMode { return paneOverlay }

func (v *todoPaneView) ensurePicker(ctx paneRenderContext) {
	if v.initialized {
		return
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = true
	v.picker = list.New(todoListItems(ctx.todos), delegate, maxInt(12, ctx.width-8), maxInt(5, minInt(14, ctx.height-4)))
	v.picker.DisableQuitKeybindings()
	v.picker.SetFilteringEnabled(false)
	v.picker.SetStatusBarItemName("task", "tasks")
	v.initialized = true
	v.syncTitle(ctx)
}

func todoListItems(items []tododomain.Item) []list.Item {
	ordered := tododomain.CloneItems(items)
	slices.SortStableFunc(ordered, func(a, b tododomain.Item) int {
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

func (v *todoPaneView) syncTitle(ctx paneRenderContext) {
	if !v.initialized {
		return
	}
	completed, _, _ := todopane.TodoCounts(ctx.todos)
	v.picker.Title = fmt.Sprintf("Tasks · %d/%d done", completed, len(ctx.todos))
}

func (v *todoPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	v.ensurePicker(ctx)
	switch message.String() {
	case "esc", "enter":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: todoInspectViewID}}
	}
	updated, cmd := v.picker.Update(message)
	v.picker = updated
	return paneKeyResult{handled: true, cmd: cmd}
}

func (v *todoPaneView) Render(ctx paneRenderContext) string {
	v.ensurePicker(ctx)
	if !v.initialized {
		return ""
	}
	v.picker.SetItems(todoListItems(ctx.todos))
	v.syncTitle(ctx)
	v.picker.SetSize(maxInt(12, ctx.width-8), maxInt(4, minInt(8, ctx.height-6)))
	v.picker.SetShowStatusBar(false)
	// TODO inspection keeps list navigation but renders contextual help in the shared footer.
	// Pagination is hidden to avoid mutable paginator presentation during resize/render.
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(false)
	return renderModalRows(ctx, promptBorder, strings.Split(v.picker.View(), "\n"))
}

func (m *bubbleModel) toggleTodoPane() {
	if m.panes.bottom.has(todoInspectViewID) {
		m.panes.bottom.remove(todoInspectViewID)
	} else {
		m.panes.bottom.push(&todoPaneView{})
	}
	m.requestRelayout()
}
