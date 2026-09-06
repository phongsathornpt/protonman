package tui

import "strings"

func (m bubbleModel) welcomeCard() string {
	width := maxInt(8, m.width-2)
	rows := []string{brandStyle.Render("Proton") + mutedStyle.Render("  "+appVersion)}
	if cwd := strings.TrimSpace(m.workDir); cwd != "" {
		rows = append(rows, mutedStyle.Render(truncateWithEllipsis(cwd, width)))
	}

	if m.activeModel != "" {
		provider := strings.TrimSpace(m.activeProvider)
		rows = append(rows, brandStyle.Render(truncateWithEllipsis(m.activeModel, width)))
		if provider != "" {
			rows = append(rows, mutedStyle.Render(truncateWithEllipsis("provider: "+provider, width)))
		}
		if m.runner != nil {
			rows = append(rows, mutedStyle.Render("Ask anything · /help"))
		}
		return strings.Join(rows, "\n")
	}

	if m.runner == nil {
		rows = append(rows,
			warningStyle.Render("No model selected"),
			mutedStyle.Render("Ctrl+P choose a model"),
		)
		return strings.Join(rows, "\n")
	}
	rows = append(rows, mutedStyle.Render("Ask anything · /help"))
	return strings.Join(rows, "\n")
}

func promptPlaceholder(hasRunner bool) string {
	if hasRunner {
		return "Ask Proton to inspect or change this workspace…"
	}
	return "Type a message or /command…"
}

func (m *bubbleModel) resetTranscript() {
	m.ensureHistoryState().Reset()
	m.syncLegacyBlocks()
	m.followTail = true
	m.showWelcome = true
	m.refreshTranscriptViewport(true)
}

// resetConversation clears both the visible transcript and provider history.
// Keeping this separate from resetTranscript makes Ctrl+L a safe display-only
// action while /new has the explicit semantics users expect from its name.
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
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
