package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
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
	completed, active, pending := todoCounts(m.todo)
	rows := []string{brandStyle.Render(fmt.Sprintf("Tasks %d/%d · %d active · %d pending", completed, len(m.todo), active, pending))}
	limit := todoInspectionLimit(m.height)
	end := minInt(len(m.todo), v.offset+limit)
	for _, item := range m.todo[v.offset:end] {
		glyph := glyphTodoPending
		style := mutedStyle
		if item.Status == tododomain.StatusInProgress {
			glyph, style = glyphTodoActive, brandStyle
		}
		if item.Status == tododomain.StatusCompleted {
			glyph, style = glyphToolSuccess, successStyle
		}
		rows = append(rows, style.Render(glyph+truncateWithEllipsis(item.Text, maxInt(12, m.width-8))))
		rows = append(rows, mutedStyle.Render("  id: "+strings.TrimSpace(item.ID)))
	}
	if len(m.todo) > limit {
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("%d-%d of %d · ↑/↓ scroll · esc close", v.offset+1, end, len(m.todo))))
	} else {
		rows = append(rows, mutedStyle.Render("esc close"))
	}
	return renderModalRows(m, promptBorder, rows)
}

func todoInspectionLimit(height int) int {
	return maxInt(1, minInt(8, (height-7)/2))
}

func (m *bubbleModel) toggleTodoPane() {
	if m.bottom.has(todoInspectViewID) {
		m.bottom.remove(todoInspectViewID)
	} else {
		m.bottom.push(&todoPaneView{})
	}
	m.relayout()
}
