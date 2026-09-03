package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/projectTHORN/proton/internal/permission"
)

func (m bubbleModel) statusView() string {
	if m.busy {
		activity := m.activity
		if !m.busyStarted.IsZero() {
			activity += " " + formatElapsed(time.Since(m.busyStarted))
		}
		return statusStyle.Render(m.spinner.View() + " " + activity)
	}
	if m.hasPermissionView() {
		return warningStyle.Render("action required · permission")
	}
	return ""
}

func (m bubbleModel) infoView() string {
	if view := m.permissionView(); view != nil {
		if view.parked {
			return mutedStyle.Render("tab return · y allow · s session · n deny · pgup scroll")
		}
		return mutedStyle.Render("j/k move · 1-3 select · y once · s session · n deny · esc read")
	}

	targetWidth := m.width - 2
	if targetWidth <= 0 {
		targetWidth = 80
	}

	parts := []string{m.modeChip()}
	if n := len(m.queue); n > 0 {
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("%d queued", n)))
	}
	if m.skills != nil {
		active := m.skills.ActivatedList()
		if len(active) == 1 {
			cleanSkill := truncateWithEllipsis(active[0], maxInt(14, targetWidth/3))
			parts = append(parts, successStyle.Render("skill: "+cleanSkill))
		} else if len(active) > 1 {
			parts = append(parts, successStyle.Render(fmt.Sprintf("%d skills active", len(active))))
		}
	}

	candidates := []string{"shift+tab mode"}
	if m.skills != nil && len(m.skills.List()) > 0 {
		candidates = append(candidates, "ctrl+s skills")
	}
	candidates = append(candidates, "ctrl+t transcript", "ctrl+l clear")

	sepStr := glyphSep
	sepWidth := ansi.StringWidth(sepStr)
	currentWidth := 0
	for i, p := range parts {
		if i > 0 {
			currentWidth += sepWidth
		}
		currentWidth += ansi.StringWidth(p)
	}

	for _, cand := range candidates {
		rendered := mutedStyle.Render(cand)
		candWidth := ansi.StringWidth(rendered) + sepWidth
		if currentWidth+candWidth <= targetWidth {
			parts = append(parts, rendered)
			currentWidth += candWidth
		}
	}

	return strings.Join(parts, mutedStyle.Render(sepStr))
}

func (m bubbleModel) modeChip() string {
	if m.planMode {
		return planStyle.Render("plan · read-only")
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
	if view := m.permissionView(); view != nil {
		return m.infoView()
	}
	if m.slashOpen() {
		return mutedStyle.Render("tab accept · enter run · esc close · ↑↓ move")
	}
	return mutedStyle.Render("enter send · ctrl+j newline · shift+tab mode · ctrl+t transcript · ctrl+c quit")
}

func (m *bubbleModel) cycleMode() {
	mode := m.service.Mode()
	switch {
	case m.planMode:
		m.setPlanEnabled(false)
		_ = m.service.SetMode(permission.ModeAlwaysApprove)
	case mode == permission.ModeAlwaysApprove:
		_ = m.service.SetMode(permission.ModeAsk)
	default:
		if mode != permission.ModeAsk && mode != permission.ModeAuto {
			_ = m.service.SetMode(permission.ModeAsk)
		}
		m.setPlanEnabled(true)
	}
}

func (m *bubbleModel) setPlanMode(argument string) {
	enabled := m.planMode
	switch strings.ToLower(argument) {
	case "":
		enabled = !enabled
	case "on", "true":
		enabled = true
	case "off", "false":
		enabled = false
	default:
		m.appendError("usage: /plan [on|off]")
		return
	}
	if enabled && m.service.Mode() != permission.ModeAsk && m.service.Mode() != permission.ModeAuto {
		_ = m.service.SetMode(permission.ModeAsk)
	}
	m.setPlanEnabled(enabled)
	state := "off"
	if m.planMode {
		state = "on (read-only)"
	}
	m.appendLine("plan mode: " + state)
}

func (m *bubbleModel) setPlanEnabled(enabled bool) {
	m.planMode = enabled
	if !enabled {
		m.service.SetCallGuard(nil)
		return
	}
	m.service.SetCallGuard(func(_ context.Context, request permission.Request) error {
		if !m.planMode {
			return nil
		}
		switch request.ToolKind {
		case permission.ToolRead, permission.ToolGrep, permission.ToolWebFetch, permission.ToolWebSearch:
			return nil
		default:
			return fmt.Errorf("plan mode is read-only; %s tool %q is blocked", request.ToolKind, request.ToolName)
		}
	})
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
	if m.bottom == nil || m.bottom.prompt() == nil {
		return ""
	}
	width := maxInt(1, m.width-2)
	border := promptBorder
	if m.bottom.bashMode() {
		border = commandColor
	}
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(width)
	return style.Render(m.bottom.prompt().View())
}
