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

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func composerUsableWidth(terminalWidth int) int {
	return maxInt(1, terminalWidth)
}

func (m *bubbleModel) promptView() string {
	if m.panes.bottom == nil || m.panes.bottom.prompt() == nil {
		return ""
	}
	usableWidth := composerUsableWidth(m.layout.width)
	border := promptBorderStyle.Render(strings.Repeat("─", usableWidth))
	return border + "\n" + m.panes.bottom.prompt().View() + "\n" + border
}
