package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
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
		if top.PresentationMode() == paneBlocking {
			return ""
		}
		if m.slashOpen() {
			return m.idleContextFooter()
		}
		// Overlay panes own keyboard focus and render their own contextual help.
		// Keep the composer visible for continuity, but do not show send/newline
		// hints that are inactive (and often wrong) while the overlay is open.
		return ""
	}
	if !m.panes.bottom.composerVisible() {
		return ""
	}
	if m.busy || !m.conversationViewport.following() || m.permissionView() != nil || m.planMode {
		return m.shortcutHint()
	}
	return m.idleContextFooter()
}

func (m *bubbleModel) idleContextFooter() string {
	const inset = " "
	width := maxInt(1, m.layout.width-len(inset)*3)
	left := "? for shortcuts"
	right := strings.TrimSpace(m.activeModel)
	if right == "" {
		right = "unselected"
	}
	right += " · " + reasoningEffortLabel(m.reasoningEffort)
	if ansi.StringWidth(left)+ansi.StringWidth(right)+2 > width {
		return inset + mutedStyle.Render(truncateWithEllipsis(right, width))
	}
	spaces := strings.Repeat(" ", width-ansi.StringWidth(left)-ansi.StringWidth(right))
	return inset + mutedStyle.Render(left+spaces+right)
}
