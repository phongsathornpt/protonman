package runtime

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *bubbleModel) closeTranscriptOverlay() {
	if m == nil {
		return
	}
	m.panes.showTranscript = false
	if m.historyState != nil {
		m.historyState.ReleaseAlternateRenderCache()
		m.historyState.ReleaseRawTextCache()
	}
	m.panes.transcript.SetContent("")
}

func (m *bubbleModel) refreshTranscriptViewport(forceTail bool) {
	if m.historyState == nil {
		return
	}
	follow := forceTail || m.panes.transcript.AtBottom()
	content := m.historyState.Raw()
	if !m.panes.rawTranscript {
		content = strings.Join(m.historyState.RenderLinesAt(maxInt(8, m.panes.transcript.Width())), "\n")
	}
	if strings.TrimSpace(content) == "" {
		content = mutedStyle.Render("No transcript yet.")
	}
	m.panes.transcript.SetContent(content)
	if follow {
		m.panes.transcript.GotoBottom()
	}
}

func (m *bubbleModel) transcriptOverlayView() string {
	mode := "rich"
	if m.panes.rawTranscript {
		mode = "raw"
	}
	header := brandStyle.Render("Transcript") + mutedStyle.Render(" · "+mode)
	footer := mutedStyle.Render("esc/ctrl+t close · r raw/rich · pgup/pgdn scroll")
	body := lipgloss.JoinVertical(lipgloss.Left, header, m.panes.transcript.View(), footer)
	width := maxInt(1, m.layout.width-6)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentAssistant).Padding(0, 1).Width(width).Render(body)
}

func (m *bubbleModel) updateTranscriptKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(message, m.keys.Transcript) {
		m.closeTranscriptOverlay()
		return m, nil
	}
	switch message.String() {
	case "esc", "q":
		m.closeTranscriptOverlay()
		return m, nil
	case "r":
		m.panes.rawTranscript = !m.panes.rawTranscript
		if m.historyState != nil {
			if m.panes.rawTranscript {
				m.historyState.ReleaseAlternateRenderCache()
			} else {
				m.historyState.ReleaseRawTextCache()
			}
		}
		m.refreshTranscriptViewport(false)
		return m, nil
	case "pgup", "pgdown", "up", "k", "down", "j", "home", "g", "end", "G":
		updated, command := m.panes.transcript.Update(message)
		m.panes.transcript = updated
		return m, command
	default:
		return m, nil
	}
}
