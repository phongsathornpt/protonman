package runtime

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *bubbleModel) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("Starting Protonman…")
	}
	base := m.liveView()
	if m.showTranscript {
		base = overlayCenter(base, m.transcriptOverlayView(), m.width, m.height)
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
	return m.viewport.View()
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
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *bubbleModel) footerView() string {
	if m == nil || m.bottom == nil {
		return ""
	}
	if top := m.bottom.top(); top != nil {
		if top.ReplacesComposer() {
			return ""
		}
		return m.shortcutHint()
	}
	if !m.bottom.composerVisible() {
		return ""
	}
	return m.shortcutHint()
}
