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
	m.showTranscript = false
	if m.historyState != nil {
		m.historyState.ReleaseAlternateRenderCache()
		m.historyState.ReleaseRawTextCache()
	}
	m.transcriptViewport.SetContent("")
}

func (m *bubbleModel) refreshTranscriptViewport(forceTail bool) {
	if m.historyState == nil {
		return
	}
	follow := forceTail || m.transcriptViewport.AtBottom()
	content := m.historyState.Raw()
	if !m.rawTranscript {
		content = strings.Join(m.historyState.RenderLinesAt(maxInt(8, m.transcriptViewport.Width())), "\n")
	}
	if strings.TrimSpace(content) == "" {
		content = mutedStyle.Render("No transcript yet.")
	}
	m.transcriptViewport.SetContent(content)
	if follow {
		m.transcriptViewport.GotoBottom()
	}
}

func (m *bubbleModel) transcriptOverlayView() string {
	mode := "rich"
	if m.rawTranscript {
		mode = "raw"
	}
	header := brandStyle.Render("Transcript") + mutedStyle.Render(" · "+mode)
	footer := mutedStyle.Render("esc/ctrl+t close · r raw/rich · pgup/pgdn scroll")
	body := lipgloss.JoinVertical(lipgloss.Left, header, m.transcriptViewport.View(), footer)
	width := maxInt(1, m.width-6)
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
		m.rawTranscript = !m.rawTranscript
		if m.historyState != nil {
			if m.rawTranscript {
				m.historyState.ReleaseAlternateRenderCache()
			} else {
				m.historyState.ReleaseRawTextCache()
			}
		}
		m.refreshTranscriptViewport(false)
		return m, nil
	case "pgup":
		m.transcriptViewport.PageUp()
		return m, nil
	case "pgdown":
		m.transcriptViewport.PageDown()
		return m, nil
	case "up", "k":
		m.transcriptViewport.ScrollUp(1)
		return m, nil
	case "down", "j":
		m.transcriptViewport.ScrollDown(1)
		return m, nil
	case "home", "g":
		m.transcriptViewport.GotoTop()
		return m, nil
	case "end", "G":
		m.transcriptViewport.GotoBottom()
		return m, nil
	default:
		return m, nil
	}
}
