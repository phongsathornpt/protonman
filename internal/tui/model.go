package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
)

const (
	defaultBubbleWidth  = 80
	defaultBubbleHeight = 24
	promptRows          = 1
)

type bubbleModel struct {
	ctx      context.Context
	service  *toolcall.Service
	registry tool.Registry
	skills   *skill.Registry
	runner   applicationturn.Runner
	bridge   *permissionBridge
	workDir  string

	viewport           viewport.Model
	transcriptViewport viewport.Model
	spinner            spinner.Model
	keys               bubbleKeyMap
	bottom             *bottomPane
	historyState       *HistoryState

	queue           []string
	todo            []TodoItem
	todoHidden      bool
	busy            bool
	activity        string
	pendingActivity string
	planMode        bool
	followTail      bool
	showWelcome     bool
	showTranscript  bool
	rawTranscript   bool
	nextID          uint64
	width           int
	height          int
	busyStarted     time.Time
	turnCancel      context.CancelFunc
	turnEvents      <-chan tea.Msg
	messages        []model.Message

	// Compatibility snapshots for existing in-package tests during the
	// migration. Runtime ownership lives in bottom/historyState.
	blocks      []Block
	prompt      *textarea.Model
	history     []string
	historyPos  int
	modal       *permissionRequest
	modalParked bool
	permIndex   int
	slashIndex  int
	bashMode    bool
}

type bubbleKeyMap struct {
	Submit     key.Binding
	Clear      key.Binding
	Quit       key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	ToggleTodo key.Binding
	Transcript key.Binding
	CycleMode  key.Binding
}

func newBubbleModel(
	ctx context.Context,
	service *toolcall.Service,
	registry tool.Registry,
	todo []TodoItem,
	runner applicationturn.Runner,
	bridge *permissionBridge,
	workDir string,
	initialMessages ...[]model.Message,
) *bubbleModel {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = brandStyle

	pane := viewport.New(defaultBubbleWidth, defaultBubbleHeight-6)
	disableViewportKeys(&pane)
	transcriptPane := viewport.New(defaultBubbleWidth-8, defaultBubbleHeight-8)
	disableViewportKeys(&transcriptPane)

	bottom := newBottomPane(runner != nil)
	messages := []model.Message(nil)
	if len(initialMessages) > 0 {
		messages = model.CloneMessages(initialMessages[0])
	}
	ui := &bubbleModel{
		ctx:                ctx,
		service:            service,
		registry:           registry,
		runner:             runner,
		bridge:             bridge,
		workDir:            workDir,
		viewport:           pane,
		transcriptViewport: transcriptPane,
		spinner:            spin,
		keys:               newBubbleKeyMap(),
		bottom:             bottom,
		historyState:       NewHistoryState(maxBubbleScrollback),
		queue:              make([]string, 0),
		todo:               append([]TodoItem{}, todo...),
		activity:           "ready",
		followTail:         true,
		showWelcome:        true,
		width:              defaultBubbleWidth,
		height:             defaultBubbleHeight,
		messages:           messages,
	}
	ui.prompt = bottom.prompt()
	ui.loadInitialMessages(messages)
	ui.syncComponentsToLegacy()
	ui.relayout()
	return ui
}

func disableViewportKeys(pane *viewport.Model) {
	pane.KeyMap.PageDown.SetEnabled(false)
	pane.KeyMap.PageUp.SetEnabled(false)
	pane.KeyMap.HalfPageUp.SetEnabled(false)
	pane.KeyMap.HalfPageDown.SetEnabled(false)
	pane.KeyMap.Up.SetEnabled(false)
	pane.KeyMap.Down.SetEnabled(false)
	pane.KeyMap.Left.SetEnabled(false)
	pane.KeyMap.Right.SetEnabled(false)
}

func newBubbleKeyMap() bubbleKeyMap {
	return bubbleKeyMap{
		Submit:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
		Clear:      key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "clear")),
		Quit:       key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		PageUp:     key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "scroll")),
		PageDown:   key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "scroll")),
		ToggleTodo: key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "todos")),
		Transcript: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "transcript")),
		CycleMode:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "mode")),
	}
}

func (m *bubbleModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.bridge.Next(), textarea.Blink)
}

func (m *bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.syncLegacyToComponents()
	defer m.syncComponentsToLegacy()

	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
		return m, nil
	case tea.KeyMsg:
		if m.showTranscript {
			return m.updateTranscriptKey(message)
		}
		return m.updateKey(message)
	case tea.MouseMsg:
		var command tea.Cmd
		if m.showTranscript {
			m.transcriptViewport, command = m.transcriptViewport.Update(message)
			return m, command
		}
		m.viewport, command = m.viewport.Update(message)
		m.followTail = m.viewport.AtBottom()
		return m, command
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		if m.busy {
			m.historyState.SetSpinnerFrame(m.spinner.View())
			m.refreshViewport()
		}
		return m, command
	case cursor.BlinkMsg:
		prompt := m.bottom.prompt()
		updated, command := prompt.Update(message)
		*prompt = updated
		return m, command
	case permissionRequestMsg:
		m.openPermission(message.request)
		m.relayout()
		return m, m.bridge.Next()
	case permissionBridgeClosedMsg:
		return m, nil
	case toolResultMsg:
		m.busy = false
		m.busyStarted = time.Time{}
		m.activity = "ready"
		m.turnCancel = nil
		m.appendToolResult(message.result, message.err)
		m.relayout()
		return m, m.drainQueue()
	case turnDeltaMsg:
		m.applyTurnEvent(message.event)
		m.refreshViewport()
		return m, waitTurnCh(m.turnEvents)
	case turnDoneMsg:
		m.busy = false
		m.busyStarted = time.Time{}
		m.activity = "ready"
		m.turnCancel = nil
		m.turnEvents = nil
		m.historyState.CommitActive()
		m.syncLegacyBlocks()
		if message.result.Message.Content != "" {
			m.messages = append(m.messages, message.result.Message)
		}
		m.appendTurnFailure(message.err)
		m.relayout()
		return m, m.drainQueue()
	}
	return m, nil
}

func (m *bubbleModel) updateKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if top := m.bottom.top(); top != nil {
		if handled, command := top.HandleKey(m, message); handled {
			m.relayout()
			return m, command
		}
	}
	if message.Type == tea.KeyShiftTab || key.Matches(message, m.keys.CycleMode) {
		m.cycleMode()
		return m, nil
	}
	if key.Matches(message, m.keys.Transcript) {
		m.showTranscript = true
		m.refreshTranscriptViewport(true)
		return m, nil
	}
	if message.String() == "ctrl+c" {
		if m.busy && m.turnCancel != nil {
			m.turnCancel()
			return m, nil
		}
		prompt := m.bottom.prompt()
		if prompt.Value() != "" || m.bottom.bashMode() {
			prompt.Reset()
			m.setBashMode(false)
			m.relayout()
			return m, nil
		}
		return m, tea.Quit
	}
	if key.Matches(message, m.keys.Clear) {
		m.resetTranscript()
		m.refreshViewport()
		return m, nil
	}
	if key.Matches(message, m.keys.ToggleTodo) {
		m.todoHidden = !m.todoHidden
		m.relayout()
		return m, nil
	}
	if key.Matches(message, m.keys.PageUp) {
		m.viewport.PageUp()
		m.followTail = m.viewport.AtBottom()
		return m, nil
	}
	if key.Matches(message, m.keys.PageDown) {
		m.viewport.PageDown()
		m.followTail = m.viewport.AtBottom()
		return m, nil
	}
	if message.String() == "tab" && m.busy {
		return m, m.submit()
	}
	prompt := m.bottom.prompt()
	if message.String() == "esc" {
		if m.bottom.bashMode() {
			m.setBashMode(false)
		}
		prompt.Reset()
		m.syncSlashView()
		m.relayout()
		return m, nil
	}
	if message.String() == "enter" {
		return m, m.submit()
	}
	if !m.bottom.bashMode() && prompt.Value() == "" && message.String() == "!" {
		m.setBashMode(true)
		return m, nil
	}
	if m.bottom.bashMode() && prompt.Value() == "" {
		switch message.Type {
		case tea.KeyBackspace, tea.KeyCtrlH, tea.KeyDelete:
			m.setBashMode(false)
			return m, nil
		}
	}
	if message.String() == "up" && (prompt.Value() == "" || m.bottom.historyNavigating()) {
		m.historyPrevious()
		m.syncSlashView()
		return m, nil
	}
	if message.String() == "down" && m.bottom.historyNavigating() {
		m.historyNext()
		m.syncSlashView()
		return m, nil
	}

	updated, command := prompt.Update(message)
	*prompt = updated
	m.syncSlashView()
	m.relayout()
	return m, command
}

func (m *bubbleModel) submit() tea.Cmd {
	m.syncLegacyToComponents()
	prompt := m.bottom.prompt()
	line := strings.TrimSpace(prompt.Value())
	if m.bottom.bashMode() {
		prompt.Reset()
		m.bottom.remove(slashViewID)
		if line == "" {
			m.setBashMode(false)
			return nil
		}
		if m.busy || m.hasPermissionView() {
			m.queue = append(m.queue, "!"+line)
			m.appendMuted(fmt.Sprintf("queued (%d): !%s", len(m.queue), line))
			m.refreshViewport()
			return nil
		}
		m.setBashMode(false)
		return m.dispatchBang(line)
	}
	if line == "" {
		return nil
	}
	prompt.Reset()
	m.bottom.remove(slashViewID)
	if m.busy || m.hasPermissionView() {
		m.queue = append(m.queue, line)
		m.appendMuted(fmt.Sprintf("queued (%d): %s", len(m.queue), line))
		m.refreshViewport()
		return nil
	}
	return m.dispatch(line)
}

func (m *bubbleModel) drainQueue() tea.Cmd {
	if m.busy || m.hasPermissionView() || len(m.queue) == 0 {
		return nil
	}
	line := m.queue[0]
	m.queue = m.queue[1:]
	if strings.HasPrefix(line, "!") && !isCommandLine(line) {
		return m.dispatchBang(strings.TrimPrefix(line, "!"))
	}
	return m.dispatch(line)
}

func (m *bubbleModel) dispatch(line string) tea.Cmd {
	m.bottom.recordHistory(line)
	if isCommandLine(line) {
		name, _, _ := splitCommand(line)
		if name != "clear" && name != "new" && name != "quit" && name != "exit" {
			m.appendUser(line)
		}
		return m.executeCommand(line)
	}
	m.appendUser(line)
	return m.startTurn(line)
}

func (m *bubbleModel) dispatchBang(command string) tea.Cmd {
	m.bottom.recordHistory("!" + command)
	m.appendUser("!" + command)
	return m.startBash(command)
}

func (m *bubbleModel) startTool(call tool.Call) tea.Cmd {
	m.busy = true
	m.busyStarted = time.Now()
	m.activity = "running " + call.Name
	m.appendToolCall(call)
	m.historyState.SetSpinnerFrame(m.spinner.View())
	m.relayout()

	ctx, cancel := context.WithCancel(m.ctx)
	m.turnCancel = cancel
	return func() tea.Msg {
		defer cancel()
		result, callErr := m.service.Call(ctx, call)
		return toolResultMsg{result: result, err: callErr}
	}
}

func (m *bubbleModel) startTurn(prompt string) tea.Cmd {
	if m.runner == nil {
		m.appendError("model client is not configured; use /help or /call")
		m.relayout()
		return nil
	}
	m.messages = append(m.messages, model.Message{Role: model.RoleUser, Content: prompt})
	m.busy = true
	m.busyStarted = time.Now()
	m.activity = "thinking"
	m.historyState.SetSpinnerFrame(m.spinner.View())
	m.historyState.StartThinking()
	m.relayout()

	ctx, cancel := context.WithCancel(m.ctx)
	m.turnCancel = cancel
	events := make(chan tea.Msg, 32)
	history := model.CloneMessages(m.messages)
	go func() {
		defer close(events)
		result, err := m.runner.Run(
			ctx,
			history,
			func(runCtx context.Context, event applicationturn.Event) error {
				select {
				case events <- turnDeltaMsg{event: event}:
					return nil
				case <-runCtx.Done():
					return runCtx.Err()
				}
			},
		)
		select {
		case events <- turnDoneMsg{result: result, err: err}:
		case <-m.ctx.Done():
		}
	}()
	m.turnEvents = events
	return waitTurnCh(events)
}

func waitTurnCh(events <-chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return nil
		}
		return msg
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
	prompt := m.bottom.prompt()
	prompt.SetWidth(maxInt(1, width-4))
	prompt.SetHeight(promptRows)
	m.transcriptViewport.Width = maxInt(20, width-10)
	m.transcriptViewport.Height = maxInt(3, height-10)
	if m.historyState != nil {
		m.historyState.InvalidateCache()
	}
	m.relayout()
	m.refreshTranscriptViewport(false)
}

func (m *bubbleModel) relayoutIfSlashChanged(bool) { m.relayout() }

func (m *bubbleModel) relayout() {
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
	parts := make([]string, 0)
	if m.showWelcome {
		parts = append(parts, m.welcomeCard())
	}
	parts = append(parts, m.renderBlocks()...)
	content := strings.Join(parts, "\n")
	follow := m.followTail || m.viewport.AtBottom()
	m.viewport.SetContent(content)
	if follow {
		m.viewport.GotoBottom()
		m.followTail = true
	}
	m.refreshTranscriptViewport(false)
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
	if m.bottom.top() != nil {
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
	m.syncLegacyBlocks()
}

type toolResultMsg struct {
	result tool.Result
	err    error
}

type turnDeltaMsg struct{ event applicationturn.Event }

type turnDoneMsg struct {
	result applicationturn.Result
	err    error
}
