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

// renderedViewport returns the memoized viewport text. It is side-effect
// free: when the cache is stale it recomputes without storing, and the
// Update boundary (primeViewCaches) performs the actual refresh.
func (m *bubbleModel) renderedViewport() string {
	if m == nil {
		return ""
	}
	if !m.viewportViewCacheFresh() {
		return m.viewport.View()
	}
	return m.viewportViewCache.content
}

func (m *bubbleModel) viewportViewCacheFresh() bool {
	cache := &m.viewportViewCache
	return cache.valid && cache.width == m.viewport.Width() && cache.height == m.viewport.Height() &&
		cache.yOffset == m.viewport.YOffset() && cache.lineCount == m.viewport.TotalLineCount()
}

func (m *bubbleModel) liveView() string {
	if m.liveViewCacheValid {
		return m.liveViewCache
	}
	return m.computeLiveView()
}

func (m *bubbleModel) computeLiveView() string {
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
	return strings.Join(parts, "\n")
}

func (m *bubbleModel) invalidateViewportView() {
	m.viewportViewCache.valid = false
	m.liveViewCacheValid = false
}

func (m *bubbleModel) invalidateLiveView() {
	m.liveViewCacheValid = false
}

// primeViewCaches refreshes memoized view content on the Update boundary so
// View and pane Render methods stay read-only (AGENTS TUI rendering
// contract). Caches start invalid; View falls back to pure computation until
// this has run at least once.
func (m *bubbleModel) primeViewCaches() {
	if m == nil {
		return
	}
	if !m.viewportViewCacheFresh() {
		cache := &m.viewportViewCache
		cache.content = m.viewport.View()
		cache.width = m.viewport.Width()
		cache.height = m.viewport.Height()
		cache.yOffset = m.viewport.YOffset()
		cache.lineCount = m.viewport.TotalLineCount()
		cache.valid = true
	}
	if !m.liveViewCacheValid {
		m.liveViewCache = m.computeLiveView()
		m.liveViewCacheValid = true
	}
}

func (m *bubbleModel) footerView() string {
	if m == nil || m.panes.bottom == nil {
		return ""
	}
	if top := m.panes.bottom.top(); top != nil {
		// Any open pane owns keyboard focus: blocking panes swallow global
		// shortcuts, and overlay panes render their own contextual help while
		// the composer stays visible for continuity. Never show send/newline
		// hints here — they are inactive (and often wrong) while a pane is open.
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
	width := max(1, m.layoutProfile().ContentWidth(m.layout.width)-1)
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
			spaces := strings.Repeat(" ", max(1, width-ansi.StringWidth(left)-ansi.StringWidth(right)))
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
	// Resolve branch/vision metadata before any frame build in this resize.
	m.primeSessionHeaderCache()
	m.layout.width = width
	m.layout.height = height
	profile := m.layoutProfile()
	m.help.SetWidth(profile.ContentWidth(width))
	prompt := m.panes.bottom.prompt()
	prompt.SetWidth(composerUsableWidth(width))
	m.panes.transcript.SetWidth(max(1, width-10))
	m.panes.transcript.SetHeight(max(1, height-10))
	if view, _ := m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView); view != nil {
		view.resize(width, height)
	}
	if view, _ := m.panes.bottom.find(providerViewID).(*providerPaneView); view != nil {
		view.resize(width, height)
	}
	if view, _ := m.panes.bottom.find(providerSelectViewID).(*providerSelectPaneView); view != nil {
		view.resize(width, height)
	}
	ctx := newPaneRenderContext(m)
	if view, _ := m.panes.bottom.find(skillsViewID).(*skillsPaneView); view != nil {
		view.resize(ctx)
	}
	if view, _ := m.panes.bottom.find(todoInspectViewID).(*todoPaneView); view != nil {
		view.resize(ctx)
	}
	if view, _ := m.panes.bottom.find(sessionResumeViewID).(*sessionResumePaneView); view != nil {
		view.resize(ctx)
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
	return mutedStyle.Render(strings.Repeat("─", max(1, width)))
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
	m.preparePaneViews()
	scroll := m.captureViewportScroll()
	m.applyFrameLayout(scroll, m.buildFrameLayout())
}

func (m *bubbleModel) preparePaneViews() {
	if m == nil || m.panes.bottom == nil {
		return
	}
	ctx := newPaneRenderContext(m)
	switch view := m.panes.bottom.top().(type) {
	case *skillsPaneView:
		view.resize(ctx)
	case *todoPaneView:
		view.resize(ctx)
	case *sessionResumePaneView:
		view.resize(ctx)
	case *modelSetupPaneView:
		view.resize(m.layout.width, m.layout.height)
	case *providerPaneView:
		view.resize(m.layout.width, m.layout.height)
	case *providerSelectPaneView:
		view.resize(m.layout.width, m.layout.height)
	}
}

func (m *bubbleModel) applyFrameLayout(scroll viewportScrollSnapshot, frame frameLayout) {
	m.layout.generation++
	frame.generation = m.layout.generation
	m.layout.frame = frame
	m.layout.geometry = panecommon.ResolveFrameGeometry(m.layout.width, m.layout.height, frame.height)
	viewportHeight := m.layout.geometry.ViewportHeight
	sizeChanged := m.viewport.Width() != m.layout.width || m.viewport.Height() != viewportHeight
	if sizeChanged {
		m.viewport.SetWidth(m.layout.width)
		m.viewport.SetHeight(viewportHeight)
		m.invalidateViewportView()
	} else {
		m.invalidateLiveView()
	}

	historyChanged := false
	if m.historyState != nil {
		committedRev, activeRev := m.historyState.Revisions()
		historyChanged = committedRev != m.conversationViewport.committedRevision ||
			activeRev != m.conversationViewport.activeRevision
	}

	if sizeChanged || historyChanged {
		m.refreshViewportWithScroll(scroll)
	}
}

func (m *bubbleModel) refreshFrameLayout() {
	if m == nil {
		return
	}
	m.preparePaneViews()
	frame := m.buildFrameLayout()
	if frame.height != m.layout.frame.height {
		m.requestRelayout()
		return
	}
	m.layout.generation++
	frame.generation = m.layout.generation
	m.layout.frame = frame
	m.invalidateLiveView()
	m.layout.geometry = panecommon.ResolveFrameGeometry(m.layout.width, m.layout.height, frame.height)
}

func (m *bubbleModel) refreshStatusFrame() {
	if m == nil {
		return
	}
	newStatus := m.statusView()
	oldStatusHeight := 0
	if m.layout.frame.status != "" {
		oldStatusHeight = lipgloss.Height(m.layout.frame.status)
	}
	newStatusHeight := 0
	if newStatus != "" {
		newStatusHeight = lipgloss.Height(newStatus)
	}
	if oldStatusHeight != newStatusHeight {
		m.requestRelayout()
		return
	}
	m.layout.generation++
	m.layout.frame.generation = m.layout.generation
	m.layout.frame.status = newStatus
	m.invalidateLiveView()
}

func (m *bubbleModel) refreshComposerFrame() {
	if m == nil {
		return
	}
	if !m.panes.bottom.composerVisible() {
		return
	}
	profile := m.layoutProfile()
	keepLowerRule := profile.Mode != panecommon.LayoutTiny || m.layout.frame.top == ""
	newComposer := composerContentView(m.promptView(), keepLowerRule)
	oldComposerHeight := 0
	if m.layout.frame.composer != "" {
		oldComposerHeight = lipgloss.Height(m.layout.frame.composer)
	}
	newComposerHeight := 0
	if newComposer != "" {
		newComposerHeight = lipgloss.Height(newComposer)
	}
	if oldComposerHeight != newComposerHeight {
		m.requestRelayout()
		return
	}
	m.layout.generation++
	m.layout.frame.generation = m.layout.generation
	m.layout.frame.composer = newComposer
	m.invalidateLiveView()
}

func (m *bubbleModel) refreshViewport() {
	m.refreshViewportWithScroll(m.captureViewportScroll())
}
