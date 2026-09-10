package runtime

import "charm.land/lipgloss/v2"

func (m *bubbleModel) resize(width int, height int) {
	if width <= 0 {
		width = defaultBubbleWidth
	}
	if height <= 0 {
		height = defaultBubbleHeight
	}
	m.layout.width = width
	m.layout.height = height
	m.help.SetWidth(maxInt(1, width-2))
	prompt := m.panes.bottom.prompt()
	prompt.SetWidth(composerUsableWidth(width))
	m.panes.transcript.SetWidth(maxInt(1, width-10))
	m.panes.transcript.SetHeight(maxInt(1, height-10))
	if view, _ := m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView); view != nil {
		view.resize(width, height)
	}
	if m.historyState != nil {
		m.historyState.SetWidth(width)
	}
	m.requestRelayout()
	m.reconcileLayout()
	m.refreshTranscriptViewport(false)
}

type layoutState struct {
	width      int
	height     int
	frame      frameChrome
	generation uint64
	dirty      bool
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
	frame.top = m.panes.bottom.renderTop(m)
	frame.footer = m.footerView()
	for _, part := range []string{frame.status, frame.top} {
		if part != "" {
			frame.height += lipgloss.Height(part)
		}
	}
	if m.panes.bottom.composerVisible() {
		frame.composer = m.promptView()
		frame.height += m.panes.bottom.prompt().Height() + 2
	}
	if frame.footer != "" {
		frame.height += lipgloss.Height(frame.footer)
	}

	return frame
}

type viewportScrollSnapshot struct {
	follow      bool
	yOffset     int
	anchor      ScrollAnchor
	anchorValid bool
}

func (m *bubbleModel) requestRelayout() {
	m.layout.dirty = true
}

func (m *bubbleModel) reconcileLayout() {
	if m == nil || !m.layout.dirty {
		return
	}
	m.layout.dirty = false
	scroll := m.captureViewportScroll()
	m.applyFrameLayout(scroll, m.buildFrameChrome())
}

func (m *bubbleModel) applyFrameLayout(scroll viewportScrollSnapshot, frame frameChrome) {
	m.layout.generation++
	frame.generation = m.layout.generation
	m.layout.frame = frame
	viewportHeight := m.layout.height - frame.height
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	if m.viewport.Width() != m.layout.width || m.viewport.Height() != viewportHeight {
		m.viewport.SetWidth(m.layout.width)
		m.viewport.SetHeight(viewportHeight)
	}
	m.refreshViewportWithScroll(scroll)
}

func (m *bubbleModel) refreshFrameChromeOnly() {
	if m == nil {
		return
	}
	frame := m.buildFrameChrome()
	if frame.height != m.layout.frame.height {
		m.requestRelayout()
		return
	}
	m.layout.generation++
	frame.generation = m.layout.generation
	m.layout.frame = frame
}

func (m *bubbleModel) refreshViewport() {
	m.refreshViewportWithScroll(m.captureViewportScroll())
}
