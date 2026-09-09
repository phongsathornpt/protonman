package runtime

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/phongsathornpt/protonman/internal/core/permission"
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
func (m *bubbleModel) promptView() string {
	if m.bottom == nil || m.bottom.prompt() == nil {
		return ""
	}
	return m.bottom.prompt().View()
}
