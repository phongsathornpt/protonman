package runtime

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

var promptBorderStyle = lipgloss.NewStyle().Foreground(promptBorder)

func promptPlaceholder(hasRunner bool, mode permission.Mode, planMode bool) string {
	if !hasRunner {
		return "Message or /command…"
	}
	// Once a runnable model is selected, keep the idle composer visually empty.
	// The context footer carries model/thinking state and the prompt glyph itself
	// is enough affordance, matching the compact reference layout.
	_ = mode
	_ = planMode
	return ""
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

func (m *bubbleModel) promptView() string {
	if m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return ""
	}
	const inset = " "
	lineWidth := maxInt(1, m.layout.width-len(inset)*2)
	border := promptBorderStyle.Render(strings.Repeat("─", lineWidth))
	return inset + border + "\n" + inset + m.panes.bottom.prompt().View() + "\n" + inset + border
}
