package runtime

import (
	"fmt"
	"io"
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

type todoReloadedMsg struct {
	snapshot tododomain.Snapshot
	err      error
}

func (m *bubbleModel) applyTodoSnapshot(snapshot tododomain.Snapshot) bool {
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

func (m *bubbleModel) syncTodoSnapshot() bool {
	if m == nil || m.todoStore == nil {
		return false
	}
	return m.applyTodoSnapshot(m.todoStore.Snapshot())
}

func (m *bubbleModel) reloadTodoSnapshotCmd() tea.Cmd {
	if m == nil || m.todoStore == nil {
		return nil
	}
	reloader, ok := m.todoStore.(tododomain.ReloadableRepository)
	if !ok {
		return nil
	}
	ctx := m.ctx
	return func() tea.Msg {
		snapshot, err := reloader.Reload(ctx)
		return todoReloadedMsg{snapshot: snapshot, err: err}
	}
}

func (m *bubbleModel) updateTodoReloaded(message todoReloadedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.appendError("refresh tasks: " + message.err.Error())
		m.refreshViewport()
		return m, nil
	}
	if m.applyTodoSnapshot(message.snapshot) {
		m.requestRelayout()
	}
	return m, nil
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

type todoSetupDelegate struct{}

func (todoSetupDelegate) Height() int                         { return 1 }
func (todoSetupDelegate) Spacing() int                        { return 0 }
func (todoSetupDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (todoSetupDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	entry, ok := item.(todoListItem)
	if !ok {
		return
	}
	prefix, style := "  ", bodyStyle
	if index == m.Index() {
		prefix, style = "> ", brandStyle
	}
	_, _ = fmt.Fprint(w, prefix+style.Render(truncateWithEllipsis(entry.Title(), maxInt(1, m.Width()-2))))
}

type todoPaneView struct {
	picker      list.Model
	initialized bool
}

func (*todoPaneView) ID() string                             { return todoInspectViewID }
func (*todoPaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (v *todoPaneView) ensurePicker(ctx paneRenderContext) {
	if v.initialized {
		return
	}
	v.picker = list.New(todoListItems(ctx.todos), todoSetupDelegate{}, maxInt(12, ctx.width-8), maxInt(5, minInt(14, ctx.height-4)))
	v.picker.DisableQuitKeybindings()
	v.picker.SetFilteringEnabled(false)
	v.picker.SetStatusBarItemName("task", "tasks")
	v.picker.SetShowTitle(false)
	v.picker.SetShowStatusBar(false)
	v.picker.SetShowPagination(false)
	v.picker.SetShowHelp(false)
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
	_ = v.picker.SetItems(todoListItems(ctx.todos))
	v.syncTitle(ctx)
	v.picker.SetSize(maxInt(12, ctx.width-8), maxInt(4, minInt(8, ctx.height-6)))
	completed, _, _ := todopane.TodoCounts(ctx.todos)
	help := ""
	if layoutModeForHeight(ctx.height) != layoutTiny {
		help = paneKeyboardHelp(ctx.width-4, "↑/↓", "Navigate", "enter", "Close", "esc", "Go Back")
	}
	items := v.picker.VisibleItems()
	start, end := paneWindow(len(items), v.picker.Index(), 7, layoutModeForHeight(ctx.height))
	listRows := make([]string, 0, end-start)
	for index := start; index < end; index++ {
		item, ok := items[index].(todoListItem)
		if !ok {
			continue
		}
		prefix, style := "  ", bodyStyle
		if index == v.picker.Index() {
			prefix, style = "> ", brandStyle
		}
		listRows = append(listRows, prefix+style.Render(truncateWithEllipsis(item.Title(), maxInt(1, ctx.width-8))))
	}
	status := fmt.Sprintf("%d/%d done", completed, len(ctx.todos))
	if selected, ok := v.picker.SelectedItem().(todoListItem); ok {
		status = selected.item.ID + " · " + status
	}
	rows := paneSection("Tasks", listRows, help, status, ctx.width)
	return renderModalRows(ctx, accentAssistant, rows)
}

func (m *bubbleModel) openTodoPane() tea.Cmd {
	if !m.panes.bottom.has(todoInspectViewID) {
		m.panes.bottom.push(&todoPaneView{})
	}
	m.requestRelayout()
	return m.reloadTodoSnapshotCmd()
}

func (m *bubbleModel) toggleTodoPane() tea.Cmd {
	if m.panes.bottom.has(todoInspectViewID) {
		m.panes.bottom.remove(todoInspectViewID)
		m.requestRelayout()
		return nil
	}
	return m.openTodoPane()
}
