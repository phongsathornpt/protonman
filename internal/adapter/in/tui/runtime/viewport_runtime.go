package runtime

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type conversationViewportMode uint8

const (
	viewportFollowing conversationViewportMode = iota
	viewportReading
)

type conversationViewportState struct {
	mode              conversationViewportMode
	tailOnly          bool
	staleTail         bool
	committedRevision uint64
	activeRevision    uint64
	lineAnchors       []ScrollAnchor
}

func (s conversationViewportState) following() bool {
	return s.mode == viewportFollowing
}

func (s *conversationViewportState) setFollowing(follow bool) {
	if follow {
		s.mode = viewportFollowing
		return
	}
	s.mode = viewportReading
}

func (m *bubbleModel) refreshViewportWithScroll(scroll viewportScrollSnapshot) {
	if m.historyState != nil && !scroll.follow && !m.conversationViewport.tailOnly {
		committedRevision, activeRevision := m.historyState.Revisions()
		if committedRevision == m.conversationViewport.committedRevision {
			// The user is reading older content and only the mutable tail changed.
			// Keep the viewport buffer stable until they scroll again instead of
			// rebuilding the entire transcript for invisible streaming deltas.
			m.conversationViewport.staleTail = activeRevision != m.conversationViewport.activeRevision
			m.conversationViewport.setFollowing(false)
			if m.showTranscript {
				m.refreshTranscriptViewport(false)
			}
			return
		}
	}

	content := ""
	tailOnly := false
	if scroll.follow && m.busy && m.historyState.Active() != nil {
		content, tailOnly = m.historyState.RenderTailContent(maxInt(1, m.viewport.Height()))
	}
	if !tailOnly {
		content = m.fullViewportContent()
	}
	m.setViewportContent(content, !tailOnly)
	m.conversationViewport.tailOnly = tailOnly
	m.restoreViewportScroll(scroll)
	if m.showTranscript {
		m.refreshTranscriptViewport(false)
	}
}

func (m *bubbleModel) setViewportContent(content string, fullHistory bool) {
	m.viewport.SetContent(content)
	m.conversationViewport.staleTail = false
	m.conversationViewport.lineAnchors = nil
	if m.historyState != nil {
		m.conversationViewport.committedRevision, m.conversationViewport.activeRevision = m.historyState.Revisions()
	}
	if !fullHistory || m.historyState == nil {
		return
	}
	historyAnchors := m.historyState.ScrollAnchors()
	if len(historyAnchors) == 0 {
		return
	}
	prefix := m.historyViewportPrefixLines()
	m.conversationViewport.lineAnchors = make([]ScrollAnchor, prefix+len(historyAnchors))
	copy(m.conversationViewport.lineAnchors[prefix:], historyAnchors)
}

func (m *bubbleModel) captureViewportScroll() viewportScrollSnapshot {
	scroll := viewportScrollSnapshot{follow: m.conversationViewport.following(), yOffset: m.viewport.YOffset()}
	if scroll.follow || m.conversationViewport.tailOnly || m.historyState == nil {
		return scroll
	}
	if m.viewport.YOffset() >= 0 && m.viewport.YOffset() < len(m.conversationViewport.lineAnchors) {
		scroll.anchor = m.conversationViewport.lineAnchors[m.viewport.YOffset()]
		_, scroll.anchorValid = m.historyState.ResolveScrollAnchor(scroll.anchor)
		if scroll.anchorValid {
			return scroll
		}
	}
	historyLine := m.viewport.YOffset() - m.historyViewportPrefixLines()
	if historyLine < 0 {
		return scroll
	}
	scroll.anchor = m.historyState.CaptureScrollAnchor(historyLine)
	_, scroll.anchorValid = m.historyState.ResolveScrollAnchor(scroll.anchor)
	return scroll
}

func (m *bubbleModel) restoreViewportScroll(scroll viewportScrollSnapshot) {
	if scroll.follow {
		m.viewport.GotoBottom()
		m.conversationViewport.setFollowing(true)
		return
	}
	m.conversationViewport.setFollowing(false)
	yOffset := scroll.yOffset
	if scroll.anchorValid && m.historyState != nil {
		if historyLine, ok := m.historyState.ResolveScrollAnchor(scroll.anchor); ok {
			yOffset = m.historyViewportPrefixLines() + historyLine
		}
	}
	if m.viewport.YOffset() != yOffset {
		m.viewport.SetYOffset(yOffset)
	}
}

func (m *bubbleModel) historyViewportPrefixLines() int {
	if !m.showWelcome || m.historyState == nil || m.historyState.RenderContent() == "" {
		return 0
	}
	return lipgloss.Height(m.welcomeCard())
}

func (m *bubbleModel) fullViewportContent() string {
	content := m.historyState.RenderContent()
	if !m.showWelcome {
		return content
	}
	welcome := m.welcomeCard()
	if content == "" {
		return welcome
	}
	return welcome + "\n" + content
}

func (m *bubbleModel) hydrateViewportForScroll() {
	if !m.conversationViewport.tailOnly && !m.conversationViewport.staleTail {
		return
	}
	scroll := m.captureViewportScroll()
	m.setViewportContent(m.fullViewportContent(), true)
	m.conversationViewport.tailOnly = false
	m.restoreViewportScroll(scroll)
}

func (m *bubbleModel) updateConversationViewport(message tea.Msg) tea.Cmd {
	m.hydrateViewportForScroll()
	updated, command := m.viewport.Update(message)
	m.viewport = updated
	m.conversationViewport.setFollowing(m.viewport.AtBottom())
	return command
}

func (m *bubbleModel) scrollConversationLines(delta int) {
	if delta == 0 {
		return
	}
	m.hydrateViewportForScroll()
	if delta < 0 {
		m.viewport.ScrollUp(-delta)
	} else {
		m.viewport.ScrollDown(delta)
	}
	m.conversationViewport.setFollowing(m.viewport.AtBottom())
}
