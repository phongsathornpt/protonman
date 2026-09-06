package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/permission"
	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
)

func (m bubbleModel) statusView() string {
	if m.hasPermissionView() {
		return warningStyle.Render(truncateWithEllipsis("action required · permission", maxInt(1, m.width-2)))
	}
	if !m.busy {
		return ""
	}

	turnAgents := m.turnAgentSnapshot()
	activeAgents, runningAgents, queuedAgents, cancelingAgents := agentActivityCounts(turnAgents)
	parts := make([]string, 0, 6)
	activity := m.activity
	if activity != "canceling" && activeAgents > 0 {
		activity = "coordinating"
	}
	if activity == "" {
		activity = "analyzing"
	}
	parts = append(parts, activity)
	if m.turnProgress.Round > 0 {
		if m.maxRounds > 0 {
			parts = append(parts, fmt.Sprintf("round %d/%d", m.turnProgress.Round, m.maxRounds))
		} else {
			parts = append(parts, fmt.Sprintf("round %d", m.turnProgress.Round))
		}
	}
	if m.turnProgress.ToolCalls > 0 {
		parts = append(parts, fmt.Sprintf("%d tools", m.turnProgress.ToolCalls))
	}
	if activeAgents > 0 {
		if activity == "canceling" {
			parts = append(parts, fmt.Sprintf("stopping %d agents", activeAgents))
		} else {
			agents := fmt.Sprintf("%d agent", activeAgents)
			if activeAgents != 1 {
				agents += "s"
			}
			parts = append(parts, agents)
			if runningAgents > 0 {
				parts = append(parts, fmt.Sprintf("%d running", runningAgents))
			}
			if queuedAgents > 0 {
				parts = append(parts, fmt.Sprintf("%d queued", queuedAgents))
			}
			if cancelingAgents > 0 {
				parts = append(parts, fmt.Sprintf("%d canceling", cancelingAgents))
			}
			if activeAgents == 1 {
				for _, st := range turnAgents {
					if st.State.Terminal() {
						continue
					}
					if activity := strings.TrimSpace(m.agentActivity[st.ID]); activity != "" {
						parts = append(parts, activity)
					}
					break
				}
			}
		}
	}
	if !m.busyStarted.IsZero() {
		parts = append(parts, formatElapsed(time.Since(m.busyStarted)))
	}
	return statusStyle.Render(truncateWithEllipsis("• "+strings.Join(parts, " · "), maxInt(1, m.width-2)))
}

func (m bubbleModel) turnAgentSnapshot() []agent.AgentStatus {
	if m.activeTurnOwner == "" {
		return m.agentSnapshot
	}
	out := make([]agent.AgentStatus, 0, len(m.agentSnapshot))
	for _, st := range m.agentSnapshot {
		if st.ParentID == m.activeTurnOwner {
			out = append(out, st)
		}
	}
	return out
}

func agentActivityCounts(snapshot []agent.AgentStatus) (active, running, queued, canceling int) {
	for _, st := range snapshot {
		switch st.State {
		case agent.StateRunning:
			running++
		case agent.StateQueued:
			queued++
		case agent.StateCanceling:
			canceling++
		}
	}
	active = running + queued + canceling
	return active, running, queued, canceling
}

func (m bubbleModel) infoView() string {
	if view := m.permissionView(); view != nil {
		if view.parked {
			return mutedStyle.Render("tab review · y once · s session · n deny")
		}
		return mutedStyle.Render("y once · s session · n deny · esc review")
	}

	targetWidth := m.width - 2
	if targetWidth <= 0 {
		targetWidth = 80
	}
	mode := layoutModeForHeight(m.height)

	parts := []string{m.modeChip()}
	if m.activeModel != "" {
		cleanModel := truncateWithEllipsis(m.activeModel, maxInt(8, targetWidth/3))
		parts = append(parts, brandStyle.Render("model: "+cleanModel))
	}
	if n := len(m.queue); n > 0 {
		parts = append(parts, mutedStyle.Render(fmt.Sprintf("%d queued", n)))
	}
	if mode == layoutNormal && m.skills != nil {
		active := m.skills.ActivatedList()
		if len(active) == 1 {
			cleanSkill := truncateWithEllipsis(active[0], maxInt(14, targetWidth/3))
			parts = append(parts, successStyle.Render("skill: "+cleanSkill))
		} else if len(active) > 1 {
			parts = append(parts, successStyle.Render(fmt.Sprintf("%d skills active", len(active))))
		}
	}

	candidates := make([]string, 0, 2)
	switch mode {
	case layoutNormal:
		candidates = append(candidates, "ctrl+p model", "/help")
	case layoutCompact:
		candidates = append(candidates, "ctrl+p model")
	}

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
	mode := permission.ModeAsk
	if m.service != nil {
		mode = m.service.Mode()
	}
	switch mode {
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
	switch layoutModeForHeight(m.height) {
	case layoutTiny:
		return mutedStyle.Render("enter send · ctrl+c")
	case layoutCompact:
		return mutedStyle.Render("enter send · ctrl+p model · ctrl+c")
	default:
		return mutedStyle.Render("enter send · ctrl+j newline · /help")
	}
}

func (m *bubbleModel) cycleMode() {
	mode := m.service.Mode()
	switch {
	case m.planMode:
		m.setPlanEnabled(false)
		_ = m.setPermissionMode(permission.ModeAlwaysApprove)
	case mode == permission.ModeAlwaysApprove:
		_ = m.setPermissionMode(permission.ModeAsk)
	default:
		if mode != permission.ModeAsk && mode != permission.ModeAuto {
			_ = m.setPermissionMode(permission.ModeAsk)
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
		_ = m.setPermissionMode(permission.ModeAsk)
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
		if m.coordinator != nil {
			m.coordinator.SetCallGuard(nil)
		}
		return
	}
	guard := func(_ context.Context, request permission.Request) error {
		if !m.planMode {
			return nil
		}
		switch request.ToolKind {
		case permission.ToolRead, permission.ToolGrep, permission.ToolWebFetch, permission.ToolWebSearch, permission.ToolTask:
			return nil
		case permission.ToolBash:
			var input struct {
				Command string `json:"command"`
			}
			if json.Unmarshal(request.Arguments, &input) == nil && tool.AnalyzeCommand(input.Command).Effect == tool.CommandEffectReadOnly {
				return nil
			}
		case permission.ToolAgent:
			switch request.ToolName {
			case "wait_agent", "get_agent", "list_agents":
				return nil
			}
		}
		return fmt.Errorf("plan mode is read-only; %s tool %q is blocked", request.ToolKind, request.ToolName)
	}
	m.service.SetCallGuard(guard)
	if m.coordinator != nil {
		m.coordinator.SetCallGuard(guard)
	}
}

func formatElapsed(duration time.Duration) string {
	if duration < time.Second {
		return "0s"
	}
	return duration.Truncate(time.Second).String()
}

func (m bubbleModel) agentsView() string {
	snapshot := m.agentSnapshot
	if m.busy && m.activeTurnOwner != "" {
		snapshot = m.turnAgentSnapshot()
	}
	if layoutModeForHeight(m.height) == layoutTiny || len(snapshot) == 0 {
		return ""
	}
	queued, running, canceling, completed := 0, 0, 0, 0
	for _, st := range snapshot {
		switch st.State {
		case agent.StateQueued:
			queued++
		case agent.StateRunning:
			running++
		case agent.StateCanceling:
			canceling++
		case agent.StateCompleted:
			completed++
		}
	}
	active := queued + running + canceling
	summary := fmt.Sprintf("Agents %d active", active)
	if running > 0 {
		summary += fmt.Sprintf(" · %d running", running)
	}
	if queued > 0 {
		summary += fmt.Sprintf(" · %d queued", queued)
	}
	if canceling > 0 {
		summary += fmt.Sprintf(" · %d canceling", canceling)
	}
	if completed > 0 {
		summary += fmt.Sprintf(" · %d done", completed)
	}
	if layoutModeForHeight(m.height) == layoutCompact || m.busy {
		return brandStyle.Render(summary)
	}

	visible := append([]agent.AgentStatus(nil), snapshot...)
	sort.SliceStable(visible, func(i, j int) bool {
		return agentDisplayPriority(visible[i].State) < agentDisplayPriority(visible[j].State)
	})
	if len(visible) > 3 {
		visible = visible[:3]
	}

	lines := []string{brandStyle.Render(summary)}
	for _, st := range visible {
		stateGlyph := glyphAgent
		style := mutedStyle
		switch st.State {
		case agent.StateCompleted:
			stateGlyph = glyphToolSuccess
			style = successStyle
		case agent.StateFailed, agent.StateCanceled:
			stateGlyph = glyphToolError
			style = errorStyle
		case agent.StateCanceling:
			style = warningStyle
		}
		elapsed := formatElapsed(agentDisplayDuration(st, time.Now()))
		detail := st.Task
		if st.State.Terminal() && strings.TrimSpace(st.Reason) != "" {
			detail = st.Reason
		} else if activity := strings.TrimSpace(m.agentActivity[st.ID]); activity != "" && !st.State.Terminal() {
			detail = activity
		}
		line := fmt.Sprintf("  %s%s · %s · %s", stateGlyph, st.ID, elapsed, truncateWithEllipsis(detail, maxInt(12, m.width-30)))
		lines = append(lines, style.Render(truncateWithEllipsis(line, maxInt(1, m.width-2))))
	}
	if more := len(snapshot) - len(visible); more > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  … %d older", more)))
	}
	return strings.Join(lines, "\n")
}

func agentDisplayPriority(state agent.State) int {
	switch state {
	case agent.StateCanceling:
		return 0
	case agent.StateRunning:
		return 1
	case agent.StateQueued:
		return 2
	case agent.StateFailed, agent.StateCanceled:
		return 3
	case agent.StateCompleted:
		return 4
	default:
		return 5
	}
}

func agentDisplayDuration(st agent.AgentStatus, now time.Time) time.Duration {
	start := st.StartTime
	if !st.StartedAt.IsZero() {
		start = st.StartedAt
	}
	if st.State.Terminal() && !st.FinishedAt.IsZero() {
		if st.StartedAt.IsZero() {
			return 0
		}
		return st.FinishedAt.Sub(st.StartedAt)
	}
	if start.IsZero() || now.Before(start) {
		return 0
	}
	return now.Sub(start)
}

func (m bubbleModel) todoView() string {
	if len(m.todo) == 0 {
		return ""
	}
	completed, active, pending := todoCounts(m.todo)
	summary := fmt.Sprintf("Tasks %d/%d", completed, len(m.todo))
	if active > 0 {
		summary += fmt.Sprintf(" · %d active", active)
	}
	if pending > 0 {
		summary += fmt.Sprintf(" · %d pending", pending)
	}
	if completed == len(m.todo) {
		summary += " ✓"
	}
	renderSummary := func(value string) string {
		return brandStyle.Render(truncateWithEllipsis(value, maxInt(1, m.width-2)))
	}
	switch layoutModeForHeight(m.height) {
	case layoutTiny, layoutCompact:
		return renderSummary(summary)
	}
	if m.busy || !m.todoExpanded {
		return renderSummary(summary + " · ctrl+o details")
	}

	limit := todoVisibleRows(m.height)
	lines := []string{renderSummary(summary)}
	shown := 0
	for _, status := range []tododomain.Status{tododomain.StatusInProgress, tododomain.StatusPending, tododomain.StatusCompleted} {
		for _, item := range m.todo {
			if item.Status != status || shown >= limit {
				continue
			}
			lines = append(lines, renderTodoItem(item, maxInt(8, m.width-6))...)
			shown++
		}
	}
	if more := len(m.todo) - shown; more > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  … %d more", more)))
	}
	return strings.Join(lines, "\n")
}

func todoCounts(items []TodoItem) (completed, active, pending int) {
	for _, item := range items {
		switch item.Status {
		case tododomain.StatusCompleted:
			completed++
		case tododomain.StatusInProgress:
			active++
		case tododomain.StatusPending:
			pending++
		}
	}
	return completed, active, pending
}

func todoVisibleRows(height int) int {
	rows := height - 18
	if rows < 4 {
		rows = 4
	}
	if rows > 10 {
		rows = 10
	}
	return rows
}

func renderTodoItem(item TodoItem, width int) []string {
	prefix := glyphTodoPending
	style := mutedStyle
	switch item.Status {
	case tododomain.StatusInProgress:
		prefix = glyphTodoActive
		style = brandStyle
	case tododomain.StatusCompleted:
		prefix = glyphToolSuccess
		style = successStyle
	}
	wrapped := wrapLines(item.Text, maxInt(1, width-2))
	out := make([]string, 0, len(wrapped))
	for i, line := range wrapped {
		if i == 0 {
			out = append(out, style.Render("  "+prefix+line))
		} else {
			out = append(out, style.Render("    "+line))
		}
	}
	return out
}

func (m *bubbleModel) appendTodo() {
	if len(m.todo) == 0 {
		m.appendLine("TODO pane is empty")
		return
	}
	m.appendLine("TODO:")
	for _, item := range m.todo {
		mark := " "
		if item.Status == tododomain.StatusInProgress {
			mark = "~"
		}
		if item.Status == tododomain.StatusCompleted {
			mark = "x"
		}
		m.appendLine(fmt.Sprintf("[%s] %s", mark, item.Text))
	}
}

func (m bubbleModel) promptView() string {
	if m.bottom == nil || m.bottom.prompt() == nil {
		return ""
	}
	if layoutModeForHeight(m.height) == layoutTiny {
		return m.bottom.prompt().View()
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
