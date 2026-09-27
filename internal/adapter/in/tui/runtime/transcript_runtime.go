package runtime

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/paneutil"
	"math"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
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
	m.panes.transcriptTailOnly = false
	m.panes.transcriptTailStale = false
	if m.historyState != nil {
		m.historyState.ReleaseAlternateRenderCache()
		m.historyState.ReleaseRawTextCache()
	}
	m.panes.transcript.SetContent("")
}

func (m *bubbleModel) refreshTranscriptViewport(forceTail bool) {
	m.refreshTranscriptViewportMode(forceTail, false)
}

func (m *bubbleModel) refreshTranscriptViewportMode(forceTail, forceRefresh bool) {
	if m.historyState == nil {
		return
	}
	follow := forceTail || m.panes.transcript.AtBottom()
	if m.busy && !follow && !forceRefresh {
		// Keep the reader's existing buffer stable while new tokens arrive
		// outside their visible region. Refresh when they return to the tail.
		m.panes.transcriptTailStale = true
		return
	}
	scrollPercent := m.panes.transcript.ScrollPercent()
	content := ""
	tailOnly := false
	if m.panes.rawTranscript {
		if m.busy && follow {
			content, tailOnly = m.historyState.RawTailContent(max(1, m.panes.transcript.Height()))
		} else {
			content = m.historyState.Raw()
		}
	} else if m.busy && follow {
		content, tailOnly = m.historyState.RenderTailContentAt(
			max(1, m.panes.transcript.Width()),
			max(1, m.panes.transcript.Height()),
		)
	} else {
		content = strings.Join(m.historyState.RenderLinesAt(max(1, m.panes.transcript.Width())), "\n")
	}
	if strings.TrimSpace(content) == "" {
		content = mutedStyle.Render("No transcript yet.")
	}
	m.panes.transcript.SetContent(content)
	m.panes.transcriptTailOnly = tailOnly
	m.panes.transcriptTailStale = false
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
	header := tuistyle.PaneTitleStyle.Render("Transcript") + mutedStyle.Render(" · "+mode)
	footer := mutedStyle.Render("esc/ctrl+t close · r raw/rich · pgup/pgdn scroll")
	body := lipgloss.JoinVertical(lipgloss.Left, header, m.panes.transcript.View(), footer)
	width := max(1, m.layout.width-6)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(panecommon.ToneColor(panecommon.ToneAssistant)).Padding(0, 1).Width(width).Render(body)
}

var transcriptRawToggleKey = key.NewBinding(key.WithKeys("r"))

func (m *bubbleModel) updateTranscriptKey(message tea.KeyPressMsg) tea.Cmd {
	if key.Matches(message, m.keys.Transcript, paneutil.Keys.Close) {
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
		m.refreshTranscriptViewportMode(false, true)
		return nil
	}
	if key.Matches(message, paneutil.Keys.Nav) {
		if m.panes.transcriptTailOnly {
			m.hydrateTranscriptViewport()
		}
		updated, command := m.panes.transcript.Update(message)
		m.panes.transcript = updated
		if m.panes.transcriptTailStale && m.panes.transcript.AtBottom() {
			m.refreshTranscriptViewport(true)
		}
		return command
	}
	return nil
}

func (m *bubbleModel) hydrateTranscriptViewport() {
	if m == nil || m.historyState == nil || !m.panes.transcriptTailOnly {
		return
	}
	content := ""
	if m.panes.rawTranscript {
		content = m.historyState.Raw()
	} else {
		content = strings.Join(m.historyState.RenderLinesAt(max(1, m.panes.transcript.Width())), "\n")
	}
	if strings.TrimSpace(content) == "" {
		content = mutedStyle.Render("No transcript yet.")
	}
	m.panes.transcript.SetContent(content)
	m.panes.transcript.GotoBottom()
	m.panes.transcriptTailOnly = false
	// Any active deltas observed during hydration are present in the snapshot.
	m.panes.transcriptTailStale = false
}
