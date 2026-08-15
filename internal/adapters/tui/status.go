package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/projectTHORN/proton/internal/domain/permission"
)

func (m bubbleModel) statusView() string {
	if m.busy {
		activity := m.activity
		if !m.busyStarted.IsZero() {
			activity += " " + formatElapsed(time.Since(m.busyStarted))
		}
		return statusStyle.Render(m.spinner.View() + " " + activity)
	}
	if m.modal != nil {
		return warningStyle.Render("waiting for permission")
	}
	return ""
}

func (m bubbleModel) infoView() string {
	if m.modal != nil {
		if m.modalParked {
			return mutedStyle.Render("tab return · y allow · s this request · n deny · pgup scroll")
		}
		return mutedStyle.Render("j/k move · 1-3 select · y once · s session · n deny · esc read")
	}
	parts := []string{m.modeChip()}
	if n := len(m.queue); n > 0 {
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("%d queued", n)))
	}
	parts = append(parts, mutedStyle.Render("shift+tab"))
	parts = append(parts, mutedStyle.Render("ctrl+l"))
	return strings.Join(parts, mutedStyle.Render(glyphSep))
}

func (m bubbleModel) modeChip() string {
	if m.planMode {
		return planStyle.Render("plan")
	}
	switch m.service.Mode() {
	case permission.ModeAlwaysApprove:
		return warningStyle.Render("always-approve")
	case permission.ModeDeny:
		return errorStyle.Render("deny")
	default:
		return mutedStyle.Render("ask")
	}
}

func (m bubbleModel) shortcutHint() string {
	if m.modal != nil {
		return m.infoView()
	}
	if m.slashOpen() {
		return mutedStyle.Render("tab accept · enter run · esc close · ↑↓ move")
	}
	return mutedStyle.Render("enter send · shift+tab mode · ctrl+l clear · ctrl+c quit")
}

func (m *bubbleModel) cycleMode() {
	mode := m.service.Mode()
	switch {
	case m.planMode:
		m.planMode = false
		_ = m.service.SetMode(permission.ModeAlwaysApprove)
	case mode == permission.ModeAlwaysApprove:
		_ = m.service.SetMode(permission.ModeAsk)
	default:
		m.planMode = true
		if mode != permission.ModeAsk && mode != permission.ModeAuto {
			_ = m.service.SetMode(permission.ModeAsk)
		}
	}
}

func (m *bubbleModel) setPlanMode(argument string) {
	switch strings.ToLower(argument) {
	case "":
		m.planMode = !m.planMode
	case "on", "true":
		m.planMode = true
	case "off", "false":
		m.planMode = false
	default:
		m.appendError("usage: /plan [on|off]")
		return
	}
	state := "off"
	if m.planMode {
		state = "on"
	}
	m.appendLine("plan mode: " + state)
}

func formatElapsed(duration time.Duration) string {
	if duration < time.Second {
		return "0s"
	}
	return duration.Truncate(time.Second).String()
}

func (m bubbleModel) todoView() string {
	if m.todoHidden || len(m.todo) == 0 {
		return ""
	}
	completed := 0
	for _, item := range m.todo {
		if item.Done {
			completed++
		}
	}
	if completed == len(m.todo) {
		return ""
	}
	visible, more := pendingFirst(m.todo, 4)
	lines := []string{brandStyle.Render(fmt.Sprintf("TODO %d/%d complete", completed, len(m.todo)))}
	for _, item := range visible {
		if item.Done {
			lines = append(lines, successStyle.Render("  ✓ "+item.Text))
			continue
		}
		lines = append(lines, mutedStyle.Render("  □ "+item.Text))
	}
	if more > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  … %d more", more)))
	}
	return strings.Join(lines, "\n")
}

func pendingFirst(items []TodoItem, limit int) ([]TodoItem, int) {
	ordered := make([]TodoItem, 0, len(items))
	for _, item := range items {
		if !item.Done {
			ordered = append(ordered, item)
		}
	}
	for _, item := range items {
		if item.Done {
			ordered = append(ordered, item)
		}
	}
	if len(ordered) <= limit {
		return ordered, 0
	}
	return ordered[:limit], len(ordered) - limit
}

func (m *bubbleModel) appendTodo() {
	if len(m.todo) == 0 {
		m.appendLine("TODO pane is empty")
		return
	}
	m.appendLine("TODO:")
	for _, item := range m.todo {
		mark := " "
		if item.Done {
			mark = "x"
		}
		m.appendLine(fmt.Sprintf("[%s] %s", mark, item.Text))
	}
}

func (m bubbleModel) promptView() string {
	width := maxInt(1, m.width-2)
	border := promptBorder
	if m.bashMode {
		border = commandColor
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(width)
	return style.Render(m.prompt.View())
}
