package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/pane"
)

const todoInspectViewID = "todo-inspect"

type todoPaneView struct{ offset int }

func (*todoPaneView) ID() string             { return todoInspectViewID }
func (*todoPaneView) ReplacesComposer() bool { return false }
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

func todoInspectionLimit(height int) int { return pane.TodoInspectionLimit(height) }

func (m *bubbleModel) toggleTodoPane() {
	if m.bottom.has(todoInspectViewID) {
		m.bottom.remove(todoInspectViewID)
	} else {
		m.bottom.push(&todoPaneView{})
	}
	m.relayout()
}
