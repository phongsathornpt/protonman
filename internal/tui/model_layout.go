package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/projectTHORN/proton/internal/tool"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
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
	prompt.SetHeight(lines)
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
	prompt := m.bottom.prompt()
	prompt.SetWidth(maxInt(1, width-4))
	m.syncPromptHeight()
	m.transcriptViewport.Width = maxInt(1, width-10)
	m.transcriptViewport.Height = maxInt(1, height-10)
	if m.historyState != nil {
		m.historyState.SetWidth(width)
	}
	m.relayout()
	m.refreshTranscriptViewport(false)
}

func (m *bubbleModel) relayoutIfSlashChanged(bool) { m.relayout() }

func (m *bubbleModel) relayout() {
	m.syncPromptHeight()
	chrome := m.chromeHeight()
	viewportHeight := m.height - chrome
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	m.viewport.Width = m.width
	m.viewport.Height = viewportHeight
	m.refreshViewport()
}

func (m *bubbleModel) chromeHeight() int {
	var height int
	if todo := m.todoView(); todo != "" {
		height += lipgloss.Height(todo)
	}
	if agents := m.agentsView(); agents != "" {
		height += lipgloss.Height(agents)
	}
	if status := m.statusView(); status != "" {
		height += lipgloss.Height(status)
	}
	if top := m.bottom.renderTop(m); top != "" {
		height += lipgloss.Height(top)
	}
	if m.bottom.composerVisible() {
		height += lipgloss.Height(m.promptView())
	}
	height += lipgloss.Height(m.footerView())
	return height
}

func (m *bubbleModel) refreshViewport() {
	follow := m.followTail || m.viewport.AtBottom()
	content := ""
	tailOnly := false
	if follow && m.busy && m.historyState.Active() != nil {
		content, tailOnly = m.historyState.RenderTailContent(maxInt(1, m.viewport.Height))
	}
	if !tailOnly {
		content = m.fullViewportContent()
	}
	m.viewport.SetContent(content)
	m.viewportTailOnly = tailOnly
	if follow {
		m.viewport.GotoBottom()
		m.followTail = true
	}
	if m.showTranscript {
		m.refreshTranscriptViewport(false)
	}
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
	if !m.viewportTailOnly {
		return
	}
	m.viewport.SetContent(m.fullViewportContent())
	m.viewport.GotoBottom()
	m.viewportTailOnly = false
}

func (m *bubbleModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Starting Proton…"
	}
	base := m.liveView()
	if m.showTranscript {
		return overlayCenter(base, m.transcriptOverlayView(), m.width, m.height)
	}
	return base
}

func (m *bubbleModel) liveView() string {
	parts := []string{m.viewport.View()}
	if todo := m.todoView(); todo != "" {
		parts = append(parts, todo)
	}
	if agents := m.agentsView(); agents != "" {
		parts = append(parts, agents)
	}
	if status := m.statusView(); status != "" {
		parts = append(parts, status)
	}
	if top := m.bottom.renderTop(m); top != "" {
		parts = append(parts, top)
	}
	if m.bottom.composerVisible() {
		parts = append(parts, m.promptView())
	}
	parts = append(parts, m.footerView())
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
	m.history = append(m.history[:0], m.bottom.composer.history...)
	m.historyPos = m.bottom.composer.historyPos
	m.bashMode = m.bottom.bashMode()
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

type toolResultMsg struct {
	call   tool.Call
	result tool.Result
	err    error
}

type turnDeltaMsg struct{ event applicationturn.Event }

type turnEventsClosedMsg struct{}

type turnDoneMsg struct {
	result applicationturn.Result
	err    error
}
