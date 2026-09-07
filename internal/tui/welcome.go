package tui

func (m bubbleModel) welcomeCard() string {
	return brandLockup(m.width)
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
