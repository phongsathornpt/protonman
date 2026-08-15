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

	"github.com/projectTHORN/proton/internal/application/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/tool"
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
	runner   applicationturn.Runner
	bridge   *permissionBridge
	workDir  string

	viewport viewport.Model
	prompt   textarea.Model
	spinner  spinner.Model
	keys     bubbleKeyMap

	blocks          []Block
	history         []string
	historyPos      int
	queue           []string
	todo            []TodoItem
	todoHidden      bool
	modal           *permissionRequest
	modalParked     bool
	permIndex       int
	slashIndex      int
	busy            bool
	activity        string
	pendingActivity string
	planMode        bool
	bashMode        bool
	followTail      bool
	showWelcome     bool
	nextID          uint64
	width           int
	height          int
	busyStarted     time.Time
	turnCancel      context.CancelFunc
	turnEvents      <-chan tea.Msg
	messages        []model.Message
}

type bubbleKeyMap struct {
	Submit     key.Binding
	Clear      key.Binding
	Quit       key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	ToggleTodo key.Binding
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
) *bubbleModel {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = statusStyle

	pane := viewport.New(defaultBubbleWidth, defaultBubbleHeight-6)
	// Viewport defaults bind letters (h/j/k/l/f/b) and space. Those belong
	// to the prompt; we drive scroll with PageUp/PageDown ourselves.
	pane.KeyMap.PageDown.SetEnabled(false)
	pane.KeyMap.PageUp.SetEnabled(false)
	pane.KeyMap.HalfPageUp.SetEnabled(false)
	pane.KeyMap.HalfPageDown.SetEnabled(false)
	pane.KeyMap.Up.SetEnabled(false)
	pane.KeyMap.Down.SetEnabled(false)
	pane.KeyMap.Left.SetEnabled(false)
	pane.KeyMap.Right.SetEnabled(false)

	ui := &bubbleModel{
		ctx:         ctx,
		service:     service,
		registry:    registry,
		runner:      runner,
		bridge:      bridge,
		workDir:     workDir,
		viewport:    pane,
		prompt:      newPrompt(runner != nil),
		spinner:     spin,
		keys:        newBubbleKeyMap(),
		blocks:      make([]Block, 0),
		history:     make([]string, 0),
		queue:       make([]string, 0),
		todo:        append([]TodoItem{}, todo...),
		activity:    "ready",
		followTail:  true,
		showWelcome: true,
		width:       defaultBubbleWidth,
		height:      defaultBubbleHeight,
		messages:    make([]model.Message, 0),
	}
	ui.relayout()
	return ui
}

func newBubbleKeyMap() bubbleKeyMap {
	return bubbleKeyMap{
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "send"),
		),
		Clear: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("ctrl+l", "clear"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "scroll"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("pgdn", "scroll"),
		),
		ToggleTodo: key.NewBinding(
			key.WithKeys("ctrl+t"),
			key.WithHelp("ctrl+t", "todos"),
		),
		CycleMode: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "mode"),
		),
	}
}

func (m *bubbleModel) Init() tea.Cmd {
	// textarea.Blink is the official chat-example Init command. Focus is
	// already set on the stored prompt in newPrompt.
	return tea.Batch(m.spinner.Tick, m.bridge.Next(), textarea.Blink)
}

func (m *bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(message)
	case tea.MouseMsg:
		var command tea.Cmd
		m.viewport, command = m.viewport.Update(message)
		m.followTail = m.viewport.AtBottom()
		return m, command
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		if m.busy {
			return m, tea.Batch(command, m.spinner.Tick)
		}
		return m, command
	case cursor.BlinkMsg:
		var command tea.Cmd
		m.prompt, command = m.prompt.Update(message)
		return m, command
	case permissionRequestMsg:
		m.modal = &message.request
		m.modalParked = false
		m.permIndex = 0
		if m.activity != "waiting for permission" {
			m.pendingActivity = m.activity
		}
		m.activity = "waiting for permission"
		return m, m.bridge.Next()
	case permissionBridgeClosedMsg:
		return m, nil
	case toolResultMsg:
		m.busy = false
		m.busyStarted = time.Time{}
		m.activity = "ready"
		m.appendToolResult(message.result, message.err)
		m.refreshViewport()
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
		if message.result.Message.Content != "" {
			m.messages = append(m.messages, message.result.Message)
		}
		if message.err != nil {
			m.appendError("turn failed: " + message.err.Error())
		}
		m.refreshViewport()
		return m, m.drainQueue()
	}
	return m, nil
}

func (m *bubbleModel) updateKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal != nil {
		return m.updatePermission(message)
	}
	if message.Type == tea.KeyShiftTab || key.Matches(message, m.keys.CycleMode) {
		m.cycleMode()
		return m, nil
	}
	if message.String() == "ctrl+c" {
		if m.busy && m.turnCancel != nil {
			m.turnCancel()
			return m, nil
		}
		if m.prompt.Value() != "" || m.bashMode {
			m.prompt.Reset()
			m.setBashMode(false)
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
	slashWasOpen := m.slashOpen()
	if slashWasOpen {
		switch message.String() {
		case "up":
			m.moveSlash(-1)
			return m, nil
		case "down":
			m.moveSlash(1)
			return m, nil
		case "tab":
			_, _ = m.acceptSlash(false)
			m.relayoutIfSlashChanged(slashWasOpen)
			return m, nil
		case "enter":
			_, command := m.acceptSlash(true)
			m.relayoutIfSlashChanged(slashWasOpen)
			return m, command
		case "esc":
			m.prompt.Reset()
			m.slashIndex = 0
			m.relayoutIfSlashChanged(slashWasOpen)
			return m, nil
		}
	}
	if message.String() == "esc" {
		if m.bashMode {
			m.setBashMode(false)
			m.prompt.Reset()
			return m, nil
		}
		m.prompt.Reset()
		return m, nil
	}
	if message.String() == "enter" {
		return m, m.submit()
	}
	if !m.bashMode && m.prompt.Value() == "" && message.String() == "!" {
		m.setBashMode(true)
		return m, nil
	}
	if m.bashMode && m.prompt.Value() == "" {
		switch message.Type {
		case tea.KeyBackspace, tea.KeyCtrlH, tea.KeyDelete:
			m.setBashMode(false)
			return m, nil
		}
	}
	if message.String() == "up" && (m.prompt.Value() == "" || m.historyPos < len(m.history)) {
		m.historyPrevious()
		return m, nil
	}
	if message.String() == "down" && m.historyPos < len(m.history) {
		m.historyNext()
		return m, nil
	}

	var command tea.Cmd
	m.prompt, command = m.prompt.Update(message)
	m.clampSlashIndex()
	m.relayoutIfSlashChanged(slashWasOpen)
	return m, command
}

func (m *bubbleModel) submit() tea.Cmd {
	line := strings.TrimSpace(m.prompt.Value())
	if m.bashMode {
		m.prompt.Reset()
		if line == "" {
			m.setBashMode(false)
			return nil
		}
		if m.busy || m.modal != nil {
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
	m.prompt.Reset()
	m.slashIndex = 0
	if m.busy || m.modal != nil {
		m.queue = append(m.queue, line)
		m.appendMuted(fmt.Sprintf("queued (%d): %s", len(m.queue), line))
		m.refreshViewport()
		return nil
	}
	return m.dispatch(line)
}

func (m *bubbleModel) drainQueue() tea.Cmd {
	if m.busy || m.modal != nil || len(m.queue) == 0 {
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
	m.history = append(m.history, line)
	m.historyPos = len(m.history)
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
	m.history = append(m.history, "!"+command)
	m.historyPos = len(m.history)
	m.appendUser("!" + command)
	return m.startBash(command)
}

func (m *bubbleModel) startTool(call tool.Call) tea.Cmd {
	m.busy = true
	m.busyStarted = time.Now()
	m.activity = "running " + call.Name
	m.appendToolRunning(call.Name)
	m.refreshViewport()
	return func() tea.Msg {
		result, callErr := m.service.Call(m.ctx, call)
		return toolResultMsg{result: result, err: callErr}
	}
}

func (m *bubbleModel) startTurn(prompt string) tea.Cmd {
	if m.runner == nil {
		m.appendError("model client is not configured; use /help or /call")
		m.refreshViewport()
		return nil
	}
	m.messages = append(m.messages, model.Message{Role: model.RoleUser, Content: prompt})
	m.busy = true
	m.busyStarted = time.Now()
	m.activity = "thinking"
	m.refreshViewport()

	ctx, cancel := context.WithCancel(m.ctx)
	m.turnCancel = cancel
	events := make(chan tea.Msg, 32)
	history := append([]model.Message{}, m.messages...)
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
		events <- turnDoneMsg{result: result, err: err}
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
	m.prompt.SetWidth(maxInt(1, width-4))
	m.prompt.SetHeight(promptRows)
	m.relayout()
}

func (m *bubbleModel) relayoutIfSlashChanged(wasOpen bool) {
	if m.slashOpen() != wasOpen {
		m.relayout()
	}
}

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
	if slash := m.slashView(); slash != "" {
		height += lipgloss.Height(slash)
	}
	height += lipgloss.Height(m.promptView())
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
}

func (m *bubbleModel) View() string {
	if m.width == 0 || m.height == 0 {
		return "Starting Proton…"
	}
	base := m.liveView()
	if m.modal == nil {
		return base
	}
	return overlayCenter(base, m.permissionCard(), m.width, m.height)
}

func (m *bubbleModel) liveView() string {
	parts := []string{m.viewport.View()}
	if todo := m.todoView(); todo != "" {
		parts = append(parts, todo)
	}
	if status := m.statusView(); status != "" {
		parts = append(parts, status)
	}
	if slash := m.slashView(); slash != "" {
		parts = append(parts, slash)
	}
	parts = append(parts, m.promptView(), m.footerView())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *bubbleModel) footerView() string {
	if m.modal != nil || m.slashOpen() {
		return m.shortcutHint()
	}
	return m.infoView()
}

type toolResultMsg struct {
	result tool.Result
	err    error
}

type turnDeltaMsg struct {
	event applicationturn.Event
}

type turnDoneMsg struct {
	result applicationturn.Result
	err    error
}
