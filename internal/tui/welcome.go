package tui

import "strings"

func (m bubbleModel) welcomeCard() string {
	title := brandStyle.Render("Proton") + mutedStyle.Render("  "+appVersion)
	if cwd := strings.TrimSpace(m.workDir); cwd != "" {
		title += mutedStyle.Render("  " + cwd)
	}
	hint := mutedStyle.Render("Ask anything · /help · ctrl+t transcript")
	if m.runner == nil {
		hint = mutedStyle.Render("No model configured · /help · /call <tool> <json>")
	}
	return title + "\n" + hint
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

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
