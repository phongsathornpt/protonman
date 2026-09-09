package runtime

import (
	"strings"

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
	}
	m.refreshViewportWithScroll(scroll)
}

func (m *bubbleModel) frameChromeForView() frameChrome {
	frame := m.buildFrameChrome()
	frame.generation = m.frameChrome.generation
	return frame
}

func (m *bubbleModel) chromeHeight() int {
	return m.buildFrameChrome().height
}

func (m *bubbleModel) refreshViewport() {
	m.refreshViewportWithScroll(m.captureViewportScroll())
}
