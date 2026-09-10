package runtime

import (
	"math"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *bubbleModel) openTranscriptOverlay() {
	if m == nil {
		return
	}
	m.panes.showTranscript = true
	m.refreshTranscriptViewport(true)
}

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
	scrollPercent := m.panes.transcript.ScrollPercent()
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
		return
	}

	// Raw and rich transcript modes can have very different line counts.
	// Preserve the reader's relative position instead of letting SetContent
	// clamp an old absolute offset to the new bottom.
	m.panes.transcript.GotoBottom()
	maxOffset := m.panes.transcript.YOffset()
	target := int(math.Round(scrollPercent * float64(maxOffset)))
	m.panes.transcript.SetYOffset(target)
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

var transcriptRawToggleKey = key.NewBinding(key.WithKeys("r"))

func (m *bubbleModel) updateTranscriptKey(message tea.KeyPressMsg) tea.Cmd {
	if key.Matches(message, m.keys.Transcript, paneKeys.Close) {
		m.closeTranscriptOverlay()
		return nil
	}
	if key.Matches(message, transcriptRawToggleKey) {
		m.panes.rawTranscript = !m.panes.rawTranscript
		if m.historyState != nil {
			if m.panes.rawTranscript {
				m.historyState.ReleaseAlternateRenderCache()
			} else {
				m.historyState.ReleaseRawTextCache()
			}
		}
		m.refreshTranscriptViewport(false)
		return nil
	}
	if key.Matches(message, paneKeys.Nav) {
		updated, command := m.panes.transcript.Update(message)
		m.panes.transcript = updated
		return command
	}
	return nil
}
