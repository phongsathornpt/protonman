package runtime

import "charm.land/lipgloss/v2"

func (m *bubbleModel) refreshViewportWithScroll(scroll viewportScrollSnapshot) {
	if m.historyState != nil && !scroll.follow && !m.viewportTailOnly {
		committedRevision, activeRevision := m.historyState.Revisions()
		if committedRevision == m.viewportCommittedRevision {
			// The user is reading older content and only the mutable tail changed.
			// Keep the viewport buffer stable until they scroll again instead of
			// rebuilding the entire transcript for invisible streaming deltas.
			m.viewportStaleTail = activeRevision != m.viewportActiveRevision
			m.followTail = false
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
	m.viewportTailOnly = tailOnly
	m.restoreViewportScroll(scroll)
	if m.showTranscript {
		m.refreshTranscriptViewport(false)
	}
}

func (m *bubbleModel) setViewportContent(content string, fullHistory bool) {
	m.viewport.SetContent(content)
	m.markViewportViewDirty()
	m.viewportStaleTail = false
	m.viewportLineAnchors = nil
	if m.historyState != nil {
		m.viewportCommittedRevision, m.viewportActiveRevision = m.historyState.Revisions()
	}
	if !fullHistory || m.historyState == nil {
		return
	}
	historyAnchors := m.historyState.ScrollAnchors()
	if len(historyAnchors) == 0 {
		return
	}
	prefix := m.historyViewportPrefixLines()
	m.viewportLineAnchors = make([]ScrollAnchor, prefix+len(historyAnchors))
	copy(m.viewportLineAnchors[prefix:], historyAnchors)
}

func (m *bubbleModel) captureViewportScroll() viewportScrollSnapshot {
	scroll := viewportScrollSnapshot{follow: m.followTail, yOffset: m.viewport.YOffset()}
	if scroll.follow || m.viewportTailOnly || m.historyState == nil {
		return scroll
	}
	if m.viewport.YOffset() >= 0 && m.viewport.YOffset() < len(m.viewportLineAnchors) {
		scroll.anchor = m.viewportLineAnchors[m.viewport.YOffset()]
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
		before := m.viewport.YOffset()
		m.viewport.GotoBottom()
		if m.viewport.YOffset() != before {
			m.markViewportViewDirty()
		}
		m.followTail = true
		return
	}
	m.followTail = false
	yOffset := scroll.yOffset
	if scroll.anchorValid && m.historyState != nil {
		if historyLine, ok := m.historyState.ResolveScrollAnchor(scroll.anchor); ok {
			yOffset = m.historyViewportPrefixLines() + historyLine
		}
	}
	if m.viewport.YOffset() != yOffset {
		m.viewport.SetYOffset(yOffset)
		m.markViewportViewDirty()
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
	if !m.viewportTailOnly && !m.viewportStaleTail {
		return
	}
	scroll := m.captureViewportScroll()
	m.setViewportContent(m.fullViewportContent(), true)
	m.viewportTailOnly = false
	m.restoreViewportScroll(scroll)
}
