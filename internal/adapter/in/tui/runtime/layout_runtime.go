package runtime

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
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
	return m.viewport.View()
}

func (m *bubbleModel) liveView() string {
	frame := m.layout.frame
	parts := []string{m.renderedViewport()}
	if frame.status != "" {
		parts = append(parts, frame.status)
	}
	top := m.panes.bottom.top()
	if top != nil && top.PresentationMode() == paneBelowComposer {
		if frame.composer != "" {
			parts = append(parts, frame.composer)
		}
		if frame.top != "" {
			parts = append(parts, frame.top)
		}
	} else {
		for _, part := range []string{frame.top, frame.composer} {
			if part != "" {
				parts = append(parts, part)
			}
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
			return ""
		}
		// Overlay panes own keyboard focus and render their own contextual help.
		// Keep the composer visible for continuity, but do not show send/newline
		// hints that are inactive (and often wrong) while the overlay is open.
		return ""
	}
	if !m.panes.bottom.composerVisible() {
		return ""
	}
	if !m.conversationViewport.following() || m.permissionView() != nil {
		return m.shortcutHint()
	}
	return m.idleContextFooter()
}

func (m *bubbleModel) idleContextFooter() string {
	const inset = " "
	width := maxInt(1, m.layout.width-len(inset)*3)
	modelName := strings.TrimSpace(m.activeModel)
	if modelName == "" {
		modelName = "unselected"
	}
	permission := m.permissionModeLabel()
	reasoning := reasoningpolicy.EffortLabel(m.reasoningEffort)
	low := m.lowConcurrencyFooterLabel()

	rightCandidates := []string{}
	if low != "" {
		rightCandidates = append(rightCandidates,
			modelName+" · "+reasoning+" · "+permission+" · "+low,
			modelName+" · "+permission+" · "+low,
			modelName+" · "+low,
			low,
		)
	} else {
		rightCandidates = append(rightCandidates,
			modelName+" · "+reasoning+" · "+permission,
			modelName+" · "+permission,
			modelName,
		)
	}
	for _, left := range []string{"? for shortcuts", "? shortcuts", "?", ""} {
		for _, right := range rightCandidates {
			available := width - ansi.StringWidth(left)
			if left != "" {
				available--
			}
			if available <= 0 {
				continue
			}
			if ansi.StringWidth(right) > available {
				if strings.Contains(right, modelName) && available >= 8 {
					right = strings.Replace(right, modelName, truncateWithEllipsis(modelName, maxInt(1, available-(ansi.StringWidth(right)-ansi.StringWidth(modelName)))), 1)
				}
			}
			if ansi.StringWidth(right) > available {
				continue
			}
			if left == "" {
				return inset + mutedStyle.Render(right)
			}
			spaces := strings.Repeat(" ", maxInt(1, width-ansi.StringWidth(left)-ansi.StringWidth(right)))
			return inset + mutedStyle.Render(left+spaces+right)
		}
	}
	return inset + mutedStyle.Render(truncateWithEllipsis(modelName, width))
}
