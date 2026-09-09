package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *bubbleModel) View() tea.View {
	if m.layout.width == 0 || m.layout.height == 0 {
		return tea.NewView("Starting Protonman…")
	}
	base := m.liveView()
	if m.panes.showTranscript {
		base = overlayCenter(base, m.transcriptOverlayView(), m.layout.width, m.layout.height)
	}
	view := tea.NewView(base)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m *bubbleModel) renderedViewport() string {
	if m == nil {
		return ""
	}
	if m.conversationViewport.renderValid {
		return m.conversationViewport.renderedViewport
	}
	m.conversationViewport.renderedViewport = m.viewport.View()
	m.conversationViewport.renderValid = true
	return m.conversationViewport.renderedViewport
}

func (m *bubbleModel) liveView() string {
	frame := m.frameChromeForView()
	parts := []string{m.renderedViewport()}
	for _, part := range []string{frame.status, frame.top, frame.composer} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if frame.footer != "" {
		parts = append(parts, frame.footer)
	}
	return strings.Join(parts, "\n")
}

func (m *bubbleModel) footerView() string {
	if m == nil || m.panes.bottom == nil {
		return ""
	}
	if top := m.panes.bottom.top(); top != nil {
		if top.ReplacesComposer() {
			return ""
		}
		return m.shortcutHint()
	}
	if !m.panes.bottom.composerVisible() {
		return ""
	}
	return m.shortcutHint()
}
