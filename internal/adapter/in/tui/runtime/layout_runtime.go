package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
	tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
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
	cache := &m.viewportViewCache
	lineCount := m.viewport.TotalLineCount()
	if cache.valid && cache.width == m.viewport.Width() && cache.height == m.viewport.Height() &&
		cache.yOffset == m.viewport.YOffset() && cache.lineCount == lineCount {
		return cache.content
	}
	cache.content = m.viewport.View()
	cache.width = m.viewport.Width()
	cache.height = m.viewport.Height()
	cache.yOffset = m.viewport.YOffset()
	cache.lineCount = lineCount
	cache.valid = true
	return cache.content
}

func (m *bubbleModel) liveView() string {
	if m.liveViewCacheValid {
		return m.liveViewCache
	}
	frame := m.layout.frame
	parts := make([]string, 0, 7)
	if frame.header != "" {
		parts = append(parts, frame.header)
	}
	parts = append(parts, m.renderedViewport())
	if frame.divider != "" {
		parts = append(parts, frame.divider)
	}
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
	m.liveViewCache = strings.Join(parts, "\n")
	m.liveViewCacheValid = true
	return m.liveViewCache
}

func (m *bubbleModel) invalidateViewportView() {
	m.viewportViewCache.valid = false
	m.liveViewCacheValid = false
}

func (m *bubbleModel) footerView() string {
	if m == nil || m.panes.bottom == nil {
		return ""
	}
	if top := m.panes.bottom.top(); top != nil {
		if top.PresentationMode() == paneBlocking {
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
	width := maxInt(1, m.layoutProfile().ContentWidth(m.layout.width)-1)
	permission := m.permissionModeLabel()
	reasoning := reasoningpolicy.EffortLabel(m.reasoningEffort)
	submitHint := m.keys.Submit.Help().Key + " send"
	newlineHint := m.keys.Newline.Help().Key + " new line"
	rightCandidates := []string{permission}
	if reasoning != "" && reasoning != permission {
		rightCandidates = append([]string{reasoning + " · " + permission}, rightCandidates...)
	}
	profile := m.layoutProfile()
	if !profile.ShowHeader && width >= 72 && strings.TrimSpace(m.activeModel) != "" {
		rightCandidates = []string{modelFooterLabel(m.activeModel)}
		if reasoning != "" {
			rightCandidates[0] += " · " + reasoning
		}
	}

	leftCandidates := []string{"? for shortcuts", "? shortcuts", "?", ""}
	if width >= 72 {
		leftCandidates = []string{
			submitHint + " · " + newlineHint + " · ? for shortcuts",
			submitHint + " · " + newlineHint,
			submitHint,
			"? for shortcuts",
			"? shortcuts",
			"?",
			"",
		}
	}
	for _, left := range leftCandidates {
		for _, right := range rightCandidates {
			available := width - ansi.StringWidth(left)
			if left != "" {
				available--
			}
			if available <= 0 || ansi.StringWidth(right) > available {
				continue
			}
			if left == "" {
				return inset + mutedStyle.Render(right)
			}
			spaces := strings.Repeat(" ", maxInt(1, width-ansi.StringWidth(left)-ansi.StringWidth(right)))
			return inset + renderIdleFooter(left, spaces, right, submitHint, newlineHint)
		}
	}
	return inset + mutedStyle.Render(truncateWithEllipsis(permission, width))
}

func renderIdleFooter(left, spaces, right, submitHint, newlineHint string) string {
	if strings.HasPrefix(left, submitHint) {
		leftView := strings.Replace(left, submitHint, userStyle.Bold(true).Render(submitHint), 1)
		leftView = strings.Replace(leftView, newlineHint, userStyle.Bold(true).Render(newlineHint), 1)
		leftView = mutedStyle.Render(leftView)
		return leftView + spaces + mutedStyle.Render(right)
	}
	return mutedStyle.Render(left + spaces + right)
}

func modelFooterLabel(modelID string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(modelID), func(r rune) bool {
		return r == '-' || r == '_'
	})
	for index, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[index] = string(runes)
	}
	return strings.Join(parts, " ")
}

func (m *bubbleModel) resize(width int, height int) {
	if width <= 0 {
		width = defaultBubbleWidth
	}
	if height <= 0 {
		height = defaultBubbleHeight
	}
	m.layout.width = width
	m.layout.height = height
	profile := m.layoutProfile()
	m.help.SetWidth(profile.ContentWidth(width))
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
	frame      frameLayout
	geometry   panecommon.FrameGeometry
	generation uint64
	dirty      bool
}

type frameLayout struct {
	generation uint64
	header     string
	divider    string
	status     string
	top        string
	composer   string
	footer     string
	height     int
}

func (m *bubbleModel) layoutProfile() panecommon.Profile {
	if m == nil {
		return panecommon.ResolveProfile(defaultBubbleWidth, defaultBubbleHeight, false)
	}
	hasBottomView := m.panes.bottom != nil && m.panes.bottom.top() != nil
	return panecommon.ResolveProfile(m.layout.width, m.layout.height, hasBottomView)
}

func (m *bubbleModel) buildFrameLayout() frameLayout {
	frame := frameLayout{}
	profile := m.layoutProfile()
	if profile.ShowHeader {
		if header := m.sessionHeaderView(); header != "" {
			frame.header = header + "\n" + chromeDivider(m.layout.width)
		}
	}
	frame.status = m.statusView()
	frame.top = m.panes.bottom.renderTop(m)
	frame.footer = m.footerView()
	if m.panes.bottom.composerVisible() {
		frame.divider = chromeDivider(m.layout.width)
		keepLowerRule := profile.Mode != panecommon.LayoutTiny || frame.top == ""
		frame.composer = composerContentView(m.promptView(), keepLowerRule)
	}
	for _, part := range []string{frame.header, frame.divider, frame.status, frame.top, frame.composer, frame.footer} {
		if part != "" {
			frame.height += lipgloss.Height(part)
		}
	}
	return frame
}

func chromeDivider(width int) string {
	return mutedStyle.Render(strings.Repeat("─", maxInt(1, width)))
}

func composerContentView(view string, keepLowerRule bool) string {
	if view == "" {
		return ""
	}
	lines := strings.Split(view, "\n")
	if len(lines) <= 2 {
		return view
	}
	end := len(lines)
	if !keepLowerRule {
		end--
	}
	return strings.Join(lines[1:end], "\n")
}

type viewportScrollSnapshot struct {
	follow      bool
	yOffset     int
	anchor      tuihistory.ScrollAnchor
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
	m.applyFrameLayout(scroll, m.buildFrameLayout())
}

func (m *bubbleModel) applyFrameLayout(scroll viewportScrollSnapshot, frame frameLayout) {
	m.layout.generation++
	frame.generation = m.layout.generation
	m.layout.frame = frame
	m.invalidateViewportView()
	m.layout.geometry = panecommon.ResolveFrameGeometry(m.layout.width, m.layout.height, frame.height)
	viewportHeight := m.layout.geometry.ViewportHeight
	if m.viewport.Width() != m.layout.width || m.viewport.Height() != viewportHeight {
		m.viewport.SetWidth(m.layout.width)
		m.viewport.SetHeight(viewportHeight)
		m.invalidateViewportView()
	}
	m.refreshViewportWithScroll(scroll)
}

func (m *bubbleModel) refreshFrameLayout() {
	if m == nil {
		return
	}
	frame := m.buildFrameLayout()
	if frame.height != m.layout.frame.height {
		m.requestRelayout()
		return
	}
	m.layout.generation++
	frame.generation = m.layout.generation
	m.layout.frame = frame
	m.invalidateViewportView()
	m.layout.geometry = panecommon.ResolveFrameGeometry(m.layout.width, m.layout.height, frame.height)
}

func (m *bubbleModel) refreshViewport() {
	m.refreshViewportWithScroll(m.captureViewportScroll())
}
