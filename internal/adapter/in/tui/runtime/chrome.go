package runtime

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func promptPlaceholder(hasRunner bool, mode permission.Mode, planMode bool) string {
	if !hasRunner {
		return "Message or /command…"
	}
	if planMode {
		return "Plan or inspect…"
	}
	switch mode {
	case permission.ModeAlwaysApprove:
		return "Message Protonman…"
	case permission.ModeDeny:
		return "Inspect workspace…"
	default:
		return "Message Protonman…"
	}
}

func (m *bubbleModel) resetTranscript() {
	m.ensureHistoryState().Reset()
	m.conversationViewport.setFollowing(true)
	m.showWelcome = true
	m.refreshTranscriptViewport(true)
}

func (m *bubbleModel) resetConversation() {
	m.ensureHistoryState().Reset()
	m.messages = nil
	m.queue = nil
	m.conversationViewport.setFollowing(true)
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
func (m *bubbleModel) promptView() string {
	if m.bottom == nil || m.bottom.prompt() == nil {
		return ""
	}
	prompt := m.bottom.prompt().View()
	meta := m.promptMetadataView()
	if meta == "" {
		return prompt
	}
	return meta + "\n" + prompt
}

func (m *bubbleModel) promptMetadataView() string {
	if m == nil {
		return ""
	}
	parts := make([]string, 0, 3)
	if model := strings.TrimSpace(m.activeModel); model != "" {
		parts = append(parts, model)
	}
	if agent := strings.TrimSpace(m.agentProfile); agent != "" {
		parts = append(parts, agent)
	}
	if workspace := formatWorkspaceDisplay(m.workDir); workspace != "" {
		parts = append(parts, workspace)
	}
	available := maxInt(1, m.layout.width-2)
	mode := m.promptModeLabel()
	if len(parts) == 0 {
		return mutedStyle.Render(truncateWithEllipsis(mode, available))
	}
	suffix := " · " + mode
	leftWidth := available - ansi.StringWidth(suffix)
	if leftWidth <= 0 {
		return mutedStyle.Render(truncateWithEllipsis(mode, available))
	}
	left := truncateWithEllipsis(strings.Join(parts, " · "), leftWidth)
	return mutedStyle.Render(left + suffix)
}

func (m *bubbleModel) promptModeLabel() string {
	if m.planMode {
		return "plan"
	}
	if m.service == nil {
		return "ask"
	}
	switch m.service.Mode() {
	case permission.ModeAlwaysApprove:
		return "auto"
	case permission.ModeDeny:
		return "deny"
	default:
		return "ask"
	}
}
