package runtime

import (
	"charm.land/bubbles/v2/key"
	"context"
	"encoding/json"
	"fmt"
	agentpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/agent"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"strings"
	"time"
)

func promptPlaceholder(hasRunner bool, mode permission.Mode, planMode bool) string {
	if !hasRunner {
		return "Type a message or /command…"
	}
	if planMode {
		return "Ask Protonman to plan or inspect (plan mode · read-only)…"
	}
	switch mode {
	case permission.ModeAlwaysApprove:
		return "Ask Protonman (auto-approve active · commands run without prompt)…"
	case permission.ModeDeny:
		return "Ask Protonman to inspect (deny mode · mutations blocked)…"
	default:
		return "Ask Protonman to inspect or change this workspace…"
	}
}

func (m *bubbleModel) resetTranscript() {
	m.ensureHistoryState().Reset()
	m.followTail = true
	m.showWelcome = true
	m.refreshTranscriptViewport(true)
}

func (m *bubbleModel) resetConversation() {
	m.ensureHistoryState().Reset()
	m.messages = nil
	m.queue = nil
	m.followTail = true
	m.showWelcome = true
	if m.skills != nil {
		m.skills.ResetActivated()
	}
	m.refreshTranscriptViewport(true)
} // resetConversation clears both the visible transcript and provider history.
// Keeping this separate from resetTranscript makes Ctrl+L a safe display-only
// action while /new has the explicit semantics users expect from its name.

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func shortcutHelp(binding key.Binding) string {
	help := binding.Help()
	return strings.TrimSpace(help.Key + " " + help.Desc)
}

func (m bubbleModel) statusView() string {
	if m.hasPermissionView() {
		return warningStyle.Render(truncateWithEllipsis("action required · permission", maxInt(1, m.width-2)))
	}
	if !m.busy {
		return ""
	}
	activeAgents, _, _, _ := agentActivityCounts(m.turnAgentSnapshot())
	activity := strings.TrimSpace(m.activity)
	if activeAgents > 0 {
		label := "agent"
		if activeAgents != 1 {
			label = "agents"
		}
		if activity == "canceling" {
			activity = fmt.Sprintf("stopping %d %s", activeAgents, label)
		} else {
			activity = fmt.Sprintf("%d %s working", activeAgents, label)
		}
	}
	if activity == "" || activity == "ready" {
		activity = "analyzing"
	}
	indicator := "● "
	if spin := m.spinner.View(); spin != "" {
		indicator = spin + " "
	}
	return statusStyle.Render(truncateWithEllipsis(indicator+activity, maxInt(1, m.width-2)))
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

func (m *bubbleModel) infoView() string {
	if view := m.permissionView(); view != nil {
		if view.parked {
			return mutedStyle.Render("tab review · y once · s session · n deny")
		}
		return mutedStyle.Render("y once · s session · n deny · esc review")
	}
	targetWidth := maxInt(1, m.width-2)
	parts := make([]string, 0, 3)
	if modelID := strings.TrimSpace(m.activeModel); modelID != "" {
		parts = append(parts, brandStyle.Render(truncateWithEllipsis(modelID, maxInt(8, targetWidth/2))))
	}
	if m.planMode {
		parts = append(parts, planStyle.Render("plan"))
	} else if m.service != nil && m.service.Mode() == permission.ModeAlwaysApprove {
		parts = append(parts, warningStyle.Render("auto"))
	}
	if m.reasoningEffort != sdk.ReasoningDefault && m.reasoningEffort != "" {
		parts = append(parts, mutedStyle.Render(string(m.reasoningEffort)))
	}
	return truncateWithEllipsis(strings.Join(parts, mutedStyle.Render(glyphSep)), targetWidth)
}

func (m *bubbleModel) modeChip() string {
	mode := permission.ModeAsk
	if m.service != nil {
		mode = m.service.Mode()
	}
	return m.modeChipFor(mode)
}

func (m *bubbleModel) modeChipFor(mode permission.Mode) string {
	if m.planMode {
		return planStyle.Render("mode: plan · read-only")
	}
	if m.width < 40 {
		switch mode {
		case permission.ModeAlwaysApprove:
			return warningStyle.Render("auto")
		case permission.ModeDeny:
			return errorStyle.Render("deny")
		default:
			return mutedStyle.Render("ask")
		}
	}
	switch mode {
	case permission.ModeAlwaysApprove:
		return warningStyle.Render("mode: auto-approve")
	case permission.ModeDeny:
		return errorStyle.Render("mode: deny")
	default:
		return mutedStyle.Render("mode: ask")
	}
}

func (m bubbleModel) shortcutHint() string {
	if view := m.permissionView(); view != nil {
		return m.infoView()
	}
	if m.slashOpen() {
		return mutedStyle.Render("tab accept · enter run · esc close · ↑↓ move")
	}
	helpView := m.help
	helpView.ShowAll = false
	helpView.SetWidth(maxInt(1, m.width-2))
	return helpView.View(m.keys)
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
	m.syncPromptPlaceholder()
	if !enabled {
		m.service.SetCallGuard(nil)
		m.agents.SetCallGuard(nil)
		return
	}
	guard := func(_ context.Context, request permission.Request) error {
		if !m.planMode {
			return nil
		}
		switch request.ToolKind {
		case permission.ToolRead, permission.ToolGrep, permission.ToolWeb, permission.ToolTask:
			return nil
		case permission.ToolBash:
			var input struct {
				Command string `json:"command"`
			}
			if json.Unmarshal(request.Arguments, &input) == nil && tool.AnalyzeCommand(input.Command).Effect == tool.CommandEffectReadOnly {
				return nil
			}
		case permission.ToolAgent:
			if request.ToolName == "subagent" {
				var input struct {
					Action string `json:"action"`
				}
				if json.Unmarshal(request.Arguments, &input) == nil && (input.Action == "wait" || input.Action == "get" || input.Action == "list") {
					return nil
				}
			}
		}
		return fmt.Errorf("plan mode is read-only; %s tool %q is blocked", request.ToolKind, request.ToolName)
	}
	m.service.SetCallGuard(guard)
	m.agents.SetCallGuard(guard)
}

func formatElapsed(duration time.Duration) string {
	return agentpane.FormatElapsed(duration)
}

func (m *bubbleModel) promptView() string {
	if m.bottom == nil || m.bottom.prompt() == nil {
		return ""
	}
	return m.bottom.prompt().View()
}
