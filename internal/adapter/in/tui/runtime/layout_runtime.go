package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *bubbleModel) syncPromptHeight() {
	if m.bottom == nil {
		return
	}
	prompt := m.bottom.prompt()
	if prompt == nil {
		return
	}
	lines := strings.Count(prompt.Value(), "\n") + 1
	if lines < 1 {
		lines = 1
	}
	if lines > 4 {
		lines = 4
	}
	if prompt.Height() != lines {
		prompt.SetHeight(lines)
	}
}

func (m *bubbleModel) resize(width int, height int) {
	if width <= 0 {
		width = defaultBubbleWidth
	}
	if height <= 0 {
		height = defaultBubbleHeight
	}
	m.width = width
	m.height = height
	m.help.SetWidth(maxInt(1, width-2))
	prompt := m.bottom.prompt()
	prompt.SetWidth(maxInt(1, width-4))
	m.syncPromptHeight()
	m.transcriptViewport.SetWidth(maxInt(1, width-10))
	m.transcriptViewport.SetHeight(maxInt(1, height-10))
	if m.historyState != nil {
		m.historyState.SetWidth(width)
	}
	m.relayout()
	m.refreshTranscriptViewport(false)
}

func (m *bubbleModel) relayoutIfSlashChanged(bool) {
	m.relayout()
}

type frameChrome struct {
	generation uint64
	status     string
	top        string
	composer   string
	footer     string
	height     int
}

func (m *bubbleModel) buildFrameChrome() frameChrome {
	frame := frameChrome{}
	frame.status = m.statusView()
	frame.top = m.bottom.renderTop(m)
	if m.bottom.composerVisible() {
		// The composer is small and stateful (cursor, focus, placeholder, bash mode).
		// Render it from the textarea model every frame instead of reusing terminal
		// output from a previous frame. Caching this string can leave stale prompt
		// rows behind when the transcript scrolls while the textarea changes.
		frame.composer = m.promptView()
	}
	frame.footer = m.footerView()
	for _, part := range []string{frame.status, frame.top, frame.composer} {
		if part != "" {
			frame.height += lipgloss.Height(part)
		}
	}
	// The footer is always joined into the live view; even an empty footer
	// occupies one physical row in lipgloss.JoinVertical.
	frame.height += lipgloss.Height(frame.footer)

	return frame
}

type viewportScrollSnapshot struct {
	follow      bool
	yOffset     int
	anchor      ScrollAnchor
	anchorValid bool
}

func (m *bubbleModel) relayout() {
	scroll := m.captureViewportScroll()
	m.syncPromptHeight()
	m.applyFrameLayout(scroll, m.buildFrameChrome())
}

func (m *bubbleModel) applyFrameLayout(scroll viewportScrollSnapshot, frame frameChrome) {
	m.layoutGeneration++
	frame.generation = m.layoutGeneration
	m.frameChrome = frame
	viewportHeight := m.height - frame.height
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	if m.viewport.Width() != m.width || m.viewport.Height() != viewportHeight {
		m.viewport.SetWidth(m.width)
		m.viewport.SetHeight(viewportHeight)
		m.markViewportViewDirty()
	}
	m.refreshViewportWithScroll(scroll)
}

func (m *bubbleModel) frameChromeForView() frameChrome {
	m.syncPromptHeight()
	frame := m.buildFrameChrome()
	if m.frameChrome.generation == 0 || frame.height != m.frameChrome.height {
		m.applyFrameLayout(m.captureViewportScroll(), frame)
		return m.frameChrome
	}
	frame.generation = m.frameChrome.generation
	m.frameChrome = frame
	return frame
}

func (m *bubbleModel) chromeHeight() int {
	return m.buildFrameChrome().height
}

func (m *bubbleModel) refreshViewport() {
	m.refreshViewportWithScroll(m.captureViewportScroll())
}

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

func (m *bubbleModel) markViewportViewDirty() {
	if m != nil {
		m.viewportViewDirty = true
	}
}

func (m *bubbleModel) renderedViewport() string {
	if m == nil {
		return ""
	}
	if !m.viewportViewDirty && m.viewportViewCache != "" {
		return m.viewportViewCache
	}
	m.viewportViewCache = m.viewport.View()
	m.viewportViewDirty = false
	return m.viewportViewCache
}

func (m *bubbleModel) liveView() string {
	frame := m.frameChromeForView()
	parts := []string{m.renderedViewport()}
	for _, part := range []string{frame.status, frame.top, frame.composer} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	parts = append(parts, frame.footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *bubbleModel) footerView() string {
	if top := m.bottom.top(); top != nil {
		if top.ReplacesComposer() {
			return ""
		}
		return m.shortcutHint()
	}
	return m.infoView()
}

func (m *bubbleModel) syncLegacyToComponents() {
	if m.bottom == nil {
		return
	}
	if m.prompt == nil {
		m.prompt = m.bottom.prompt()
	}
	if m.modal != nil && !m.hasPermissionView() {
		m.openPermission(*m.modal)
		if view := m.permissionView(); view != nil {
			view.parked = m.modalParked
			view.index = m.permIndex
		}
	}
}

func (m *bubbleModel) syncComponentsToLegacy() {
	if m.bottom == nil {
		return
	}
	m.prompt = m.bottom.prompt()
	if view := m.slashState(); view != nil {
		m.slashIndex = view.index
	} else {
		m.slashIndex = 0
	}
	if view := m.permissionView(); view != nil {
		pending := view.pending
		m.modal = &pending
		m.modalParked = view.parked
		m.permIndex = view.index
	} else {
		m.modal = nil
		m.modalParked = false
		m.permIndex = 0
	}
}
