package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/application/turn"
	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

const (
	maxBubbleScrollback = 1000
	defaultBubbleWidth  = 80
	defaultBubbleHeight = 24
)

var (
	accentColor = lipgloss.AdaptiveColor{
		Light: "#0F766E",
		Dark:  "#5EEAD4",
	}
	mutedColor = lipgloss.AdaptiveColor{
		Light: "#64748B",
		Dark:  "#94A3B8",
	}
	warningColor = lipgloss.AdaptiveColor{
		Light: "#B45309",
		Dark:  "#FBBF24",
	}
	successColor = lipgloss.AdaptiveColor{
		Light: "#15803D",
		Dark:  "#4ADE80",
	}
	errorColor = lipgloss.AdaptiveColor{
		Light: "#B91C1C",
		Dark:  "#F87171",
	}
)

var (
	brandStyle   = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	mutedStyle   = lipgloss.NewStyle().Foreground(mutedColor)
	statusStyle  = lipgloss.NewStyle().Foreground(accentColor)
	warningStyle = lipgloss.NewStyle().Foreground(warningColor)
	successStyle = lipgloss.NewStyle().Foreground(successColor)
	errorStyle   = lipgloss.NewStyle().Foreground(errorColor)
	modalStyle   = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(warningColor).
			Padding(1, 2)
)

// BubbleTeaOption configures the Bubble Tea fullscreen adapter.
type BubbleTeaOption func(*BubbleTeaUI) error

// TurnRunner is the optional provider-neutral model loop used by prompt input.
type TurnRunner interface {
	Run(context.Context, []model.Message, applicationturn.Sink) (applicationturn.Result, error)
}

// WithBubbleTeaRunner connects ordinary prompt input to the model/tool loop.
func WithBubbleTeaRunner(runner TurnRunner) BubbleTeaOption {
	return func(ui *BubbleTeaUI) error {
		ui.runner = runner
		return nil
	}
}

// BubbleTeaUI is the Bubble Tea terminal adapter over Proton services.
type BubbleTeaUI struct {
	service  *toolcall.Service
	registry tool.Registry
	todo     []TodoItem
	runner   TurnRunner
	bridge   *permissionBridge
}

// NewBubbleTea creates the component-based fullscreen TUI.
func NewBubbleTea(
	service *toolcall.Service,
	registry tool.Registry,
	todo []TodoItem,
	options ...BubbleTeaOption,
) (*BubbleTeaUI, error) {
	if service == nil {
		return nil, errors.New("Bubble Tea UI service is required")
	}
	if registry == nil {
		return nil, errors.New("Bubble Tea UI registry is required")
	}
	ui := &BubbleTeaUI{
		service:  service,
		registry: registry,
		todo:     append([]TodoItem{}, todo...),
		bridge:   newPermissionBridge(),
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(ui); err != nil {
			return nil, err
		}
	}
	return ui, nil
}

// PermissionPrompt adapts a synchronous service permission request into a
// Bubble Tea modal request/response exchange.
func (ui *BubbleTeaUI) PermissionPrompt(
	ctx context.Context,
	request permission.Request,
) (permission.Resolution, error) {
	return ui.bridge.Prompt(ctx, request)
}

// Run starts Bubble Tea with raw input, alternate-screen rendering, and mouse
// cell motion. Bubble Tea owns terminal restoration even on program failure.
func (ui *BubbleTeaUI) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start Bubble Tea UI: %w", err)
	}
	ui.service.SetPrompt(ui.PermissionPrompt)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer ui.bridge.Close()

	model := newBubbleModel(runCtx, ui.service, ui.registry, ui.todo, ui.runner, ui.bridge)
	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithContext(runCtx),
	)
	if _, err := program.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("run Bubble Tea UI: %w", ctxErr)
		}
		return fmt.Errorf("run Bubble Tea UI: %w", err)
	}
	return nil
}

type permissionBridge struct {
	requests chan permissionRequest
	done     chan struct{}
	once     sync.Once
}

type permissionRequest struct {
	request  permission.Request
	response chan permissionResponse
}

type permissionResponse struct {
	resolution permission.Resolution
	err        error
}

func newPermissionBridge() *permissionBridge {
	return &permissionBridge{
		requests: make(chan permissionRequest),
		done:     make(chan struct{}),
	}
}

func (b *permissionBridge) Prompt(
	ctx context.Context,
	request permission.Request,
) (permission.Resolution, error) {
	response := make(chan permissionResponse, 1)
	pending := permissionRequest{
		request:  request,
		response: response,
	}
	select {
	case b.requests <- pending:
	case <-ctx.Done():
		return permission.Resolution{}, fmt.Errorf("permission prompt canceled: %w", ctx.Err())
	case <-b.done:
		return permission.Resolution{}, errors.New("permission prompt closed")
	}
	select {
	case result := <-response:
		return result.resolution, result.err
	case <-ctx.Done():
		return permission.Resolution{}, fmt.Errorf("permission prompt canceled: %w", ctx.Err())
	case <-b.done:
		return permission.Resolution{}, errors.New("permission prompt closed")
	}
}

func (b *permissionBridge) Next() tea.Cmd {
	return func() tea.Msg {
		select {
		case request := <-b.requests:
			return permissionRequestMsg{request: request}
		case <-b.done:
			return permissionBridgeClosedMsg{}
		}
	}
}

func (b *permissionBridge) Close() {
	b.once.Do(func() { close(b.done) })
}

type bubbleModel struct {
	ctx      context.Context
	service  *toolcall.Service
	registry tool.Registry
	runner   TurnRunner
	bridge   *permissionBridge

	viewport viewport.Model
	prompt   textarea.Model
	spinner  spinner.Model
	help     help.Model
	keys     bubbleKeyMap

	scrollback []string
	history    []string
	historyPos int
	todo       []TodoItem
	modal      *permissionRequest
	busy       bool
	activity   string
	planMode   bool
	nextID     uint64
	width      int
	height     int
}

type bubbleKeyMap struct {
	Clear    key.Binding
	Quit     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
}

func (k bubbleKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Clear, k.Quit, k.PageUp, k.PageDown}
}

func (k bubbleKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Clear, k.Quit}, {k.PageUp, k.PageDown}}
}

func newBubbleModel(
	ctx context.Context,
	service *toolcall.Service,
	registry tool.Registry,
	todo []TodoItem,
	runner TurnRunner,
	bridge *permissionBridge,
) bubbleModel {
	prompt := textarea.New()
	prompt.Prompt = "❯ "
	prompt.Placeholder = "Ask Proton to inspect or change this workspace…"
	prompt.CharLimit = 20_000
	prompt.ShowLineNumbers = false
	prompt.EndOfBufferCharacter = ' '
	prompt.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(accentColor)
	prompt.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
		Light: "#0F172A",
		Dark:  "#F8FAFC",
	})
	prompt.BlurredStyle = prompt.FocusedStyle

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(accentColor)

	return bubbleModel{
		ctx:        ctx,
		service:    service,
		registry:   registry,
		runner:     runner,
		bridge:     bridge,
		viewport:   viewport.New(defaultBubbleWidth, defaultBubbleHeight-6),
		prompt:     prompt,
		spinner:    spin,
		help:       help.New(),
		keys:       newBubbleKeyMap(),
		scrollback: make([]string, 0),
		history:    make([]string, 0),
		todo:       append([]TodoItem{}, todo...),
		activity:   "idle",
		width:      defaultBubbleWidth,
		height:     defaultBubbleHeight,
	}
}

func newBubbleKeyMap() bubbleKeyMap {
	return bubbleKeyMap{
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
	}
}

func (m bubbleModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.bridge.Next(), m.prompt.Focus())
}

func (m bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch message := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(message)
	case tea.MouseMsg:
		var command tea.Cmd
		m.viewport, command = m.viewport.Update(message)
		return m, command
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		if m.busy {
			return m, tea.Batch(command, m.spinner.Tick)
		}
		return m, command
	case permissionRequestMsg:
		m.modal = &message.request
		m.activity = "waiting for permission"
		return m, m.bridge.Next()
	case permissionBridgeClosedMsg:
		return m, nil
	case toolResultMsg:
		m.busy = false
		m.activity = "idle"
		m.appendToolResult(message.result, message.err)
		return m, nil
	case turnResultMsg:
		m.busy = false
		m.activity = "idle"
		m.appendTurnResult(message.events, message.result, message.err)
		return m, nil
	}
	return m, nil
}

func (m bubbleModel) updateKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modal != nil {
		return m.updatePermission(message)
	}
	if message.String() == "ctrl+c" {
		if m.prompt.Value() != "" {
			m.prompt.Reset()
			return m, nil
		}
		return m, tea.Quit
	}
	if key.Matches(message, m.keys.Clear) {
		m.scrollback = make([]string, 0)
		m.refreshViewport()
		return m, nil
	}
	if key.Matches(message, m.keys.PageUp) {
		m.viewport.PageUp()
		return m, nil
	}
	if key.Matches(message, m.keys.PageDown) {
		m.viewport.PageDown()
		return m, nil
	}
	if message.String() == "esc" {
		m.prompt.Reset()
		return m, nil
	}
	if message.String() == "enter" {
		return m, m.submit()
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
	return m, command
}

func (m bubbleModel) updatePermission(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	var resolution permission.Resolution
	switch message.String() {
	case "y":
		resolution = permission.Resolution{
			Action: permission.ActionAllow,
			Reason: "user allowed one call",
		}
	case "s":
		resolution = permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeSession,
			Reason: "user allowed this exact request for the session",
		}
	case "n", "esc", "ctrl+c":
		resolution = permission.Resolution{
			Action: permission.ActionDeny,
			Reason: "user denied one call",
		}
	default:
		return m, nil
	}
	m.modal.response <- permissionResponse{resolution: resolution}
	m.modal = nil
	m.activity = "running tool"
	return m, nil
}

func (m *bubbleModel) submit() tea.Cmd {
	line := strings.TrimSpace(m.prompt.Value())
	m.prompt.Reset()
	if line == "" || m.busy {
		return nil
	}
	m.history = append(m.history, line)
	m.historyPos = len(m.history)
	m.appendLine("> " + line)
	if !strings.HasPrefix(line, ":") {
		return m.startTurn(line)
	}
	return m.executeCommand(line)
}

func (m *bubbleModel) executeCommand(line string) tea.Cmd {
	parts := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(line, ":")), " ", 3)
	command := strings.TrimSpace(parts[0])
	argument := ""
	if len(parts) > 1 {
		argument = strings.TrimSpace(parts[1])
	}
	switch command {
	case "help":
		m.appendLine(":call <tool> <json> | :tools | :mode <mode> | :plan [on|off] | :todo | :quit")
	case "tools":
		m.appendLine("Registered tools:")
		for _, definition := range m.registry.Definitions() {
			m.appendLine(fmt.Sprintf("- %s [%s]: %s", definition.Name, definition.Kind, definition.Description))
		}
	case "mode":
		if argument == "" {
			m.appendLine("permission mode: " + m.service.Mode().String())
			return nil
		}
		mode, err := permission.ParseMode(argument)
		if err != nil {
			m.appendLine("error: " + err.Error())
			return nil
		}
		if err := m.service.SetMode(mode); err != nil {
			m.appendLine("error: " + err.Error())
			return nil
		}
		m.appendLine("permission mode: " + mode.String())
	case "plan":
		m.setPlanMode(argument)
	case "todo":
		m.appendTodo()
	case "clear":
		m.scrollback = make([]string, 0)
	case "call":
		return m.startCall(parts)
	case "quit", "exit":
		return tea.Quit
	default:
		m.appendLine(fmt.Sprintf("error: unknown command %q; try :help", command))
	}
	m.refreshViewport()
	return nil
}

func (m *bubbleModel) startCall(parts []string) tea.Cmd {
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		m.appendLine("error: usage: :call <tool> <json>")
		return nil
	}
	arguments := "{}"
	if len(parts) == 3 && strings.TrimSpace(parts[2]) != "" {
		arguments = parts[2]
	}
	m.nextID++
	call, err := tool.NewCall(
		fmt.Sprintf("bubble-%d", m.nextID),
		strings.TrimSpace(parts[1]),
		[]byte(arguments),
	)
	if err != nil {
		m.appendLine("error: " + err.Error())
		return nil
	}
	m.busy = true
	m.activity = "running " + call.Name
	m.appendLine("tool running: " + call.Name)
	m.refreshViewport()
	return func() tea.Msg {
		result, callErr := m.service.Call(m.ctx, call)
		return toolResultMsg{result: result, err: callErr}
	}
}

func (m *bubbleModel) startTurn(prompt string) tea.Cmd {
	if m.runner == nil {
		m.appendLine("error: model client is not configured")
		m.refreshViewport()
		return nil
	}
	m.busy = true
	m.activity = "thinking"
	m.refreshViewport()
	return func() tea.Msg {
		events := make([]applicationturn.Event, 0)
		result, err := m.runner.Run(
			m.ctx,
			[]model.Message{{Role: model.RoleUser, Content: prompt}},
			func(_ context.Context, event applicationturn.Event) error {
				events = append(events, event)
				return nil
			},
		)
		return turnResultMsg{events: events, result: result, err: err}
	}
}

func (m *bubbleModel) appendToolResult(result tool.Result, err error) {
	if result.Output != "" {
		for _, line := range strings.Split(strings.TrimSuffix(result.Output, "\n"), "\n") {
			m.appendLine("output: " + line)
		}
	}
	if result.CheckpointID != "" {
		m.appendLine("checkpoint: " + result.CheckpointID)
	}
	if err != nil {
		if result.Failure != nil {
			m.appendLine(fmt.Sprintf("error [%s]: %s", result.Failure.Code, result.Failure.Message))
		} else {
			m.appendLine("error: " + err.Error())
		}
		m.refreshViewport()
		return
	}
	m.appendLine("tool completed")
	m.refreshViewport()
}

func (m *bubbleModel) appendTurnResult(
	events []applicationturn.Event,
	result applicationturn.Result,
	err error,
) {
	for _, event := range events {
		switch event.Kind {
		case applicationturn.EventTextDelta:
			m.appendLine("assistant: " + event.Text)
		case applicationturn.EventToolCall:
			m.appendLine("tool requested: " + event.Call.Name)
		case applicationturn.EventToolResult:
			m.appendLine("tool finished: " + event.Call.Name)
			if event.Result.Output != "" {
				m.appendLine("output: " + event.Result.Output)
			}
		}
	}
	if result.Message.Content != "" {
		m.appendLine("assistant: " + result.Message.Content)
	}
	if err != nil {
		m.appendLine("turn failed: " + err.Error())
	}
	m.refreshViewport()
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
	todoRows := todoHeight(m.todo, width)
	viewportHeight := height - todoRows - 9
	if viewportHeight < 1 {
		viewportHeight = 1
	}
	m.viewport.Width = width
	m.viewport.Height = viewportHeight
	m.prompt.SetWidth(maxInt(1, width-4))
	m.prompt.SetHeight(3)
	m.help.Width = width
	m.refreshViewport()
}

func (m *bubbleModel) refreshViewport() {
	content := strings.Join(m.scrollback, "\n")
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m *bubbleModel) setPlanMode(argument string) {
	switch strings.ToLower(argument) {
	case "":
		m.planMode = !m.planMode
	case "on", "true":
		m.planMode = true
	case "off", "false":
		m.planMode = false
	default:
		m.appendLine("error: usage: :plan [on|off]")
		return
	}
	state := "off"
	if m.planMode {
		state = "on"
	}
	m.appendLine("plan mode: " + state)
}

func (m *bubbleModel) appendTodo() {
	if len(m.todo) == 0 {
		m.appendLine("TODO pane is empty")
		return
	}
	m.appendLine("TODO:")
	for _, item := range m.todo {
		mark := " "
		if item.Done {
			mark = "x"
		}
		m.appendLine(fmt.Sprintf("[%s] %s", mark, item.Text))
	}
}

func (m *bubbleModel) appendLine(line string) {
	clean := sanitizeBubbleText(line)
	m.scrollback = append(m.scrollback, clean)
	if len(m.scrollback) > maxBubbleScrollback {
		m.scrollback = m.scrollback[len(m.scrollback)-maxBubbleScrollback:]
	}
}

func (m *bubbleModel) historyPrevious() {
	if len(m.history) == 0 || m.historyPos == 0 {
		return
	}
	m.historyPos--
	m.prompt.SetValue(m.history[m.historyPos])
	m.prompt.CursorEnd()
}

func (m *bubbleModel) historyNext() {
	if m.historyPos >= len(m.history) {
		return
	}
	m.historyPos++
	if m.historyPos == len(m.history) {
		m.prompt.Reset()
		return
	}
	m.prompt.SetValue(m.history[m.historyPos])
	m.prompt.CursorEnd()
}

func (m bubbleModel) View() string {
	if m.modal != nil {
		return m.modalView()
	}
	if m.width == 0 || m.height == 0 {
		return "Starting Proton…"
	}
	return m.liveView()
}

func (m bubbleModel) liveView() string {
	brand := brandStyle.Render("PROTON") + mutedStyle.Render("  Go coding agent")
	status := m.statusView()
	todo := m.todoView()
	prompt := m.promptView()
	info := m.infoView()
	helpView := m.help.View(m.keys)
	parts := []string{brand, m.viewport.View()}
	if todo != "" {
		parts = append(parts, todo)
	}
	parts = append(parts, status, prompt, info, mutedStyle.Render(helpView))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m bubbleModel) modalView() string {
	request := m.modal.request
	body := strings.Join([]string{
		warningStyle.Render("Permission required"),
		fmt.Sprintf("%s (%s)", request.ToolName, request.ToolKind),
		mutedStyle.Render("Target: " + request.Detail),
		"",
		"y allow once   s allow for session   n deny",
	}, "\n")
	modal := modalStyle.Render(body)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}

func (m bubbleModel) statusView() string {
	activity := m.activity
	if m.busy {
		activity = m.spinner.View() + " " + activity
	}
	return statusStyle.Render("· " + activity + " · permission: " + m.service.Mode().String())
}

func (m bubbleModel) todoView() string {
	if len(m.todo) == 0 {
		return ""
	}
	completed := 0
	for _, item := range m.todo {
		if item.Done {
			completed++
		}
	}
	lines := []string{brandStyle.Render(fmt.Sprintf("TODO %d/%d complete", completed, len(m.todo)))}
	visible := len(m.todo)
	if visible > 4 {
		visible = 4
	}
	for _, item := range m.todo[:visible] {
		if item.Done {
			lines = append(lines, successStyle.Render("  ✓ "+item.Text))
			continue
		}
		lines = append(lines, mutedStyle.Render("  □ "+item.Text))
	}
	if len(m.todo) > visible {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("  … %d more", len(m.todo)-visible)))
	}
	return strings.Join(lines, "\n")
}

func (m bubbleModel) promptView() string {
	width := maxInt(1, m.width-2)
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(0, 1).
		Width(width)
	return style.Render(m.prompt.View())
}

func (m bubbleModel) infoView() string {
	plan := "plan off"
	if m.planMode {
		plan = "plan"
	}
	return mutedStyle.Render("proton · " + plan + " · :help")
}

func todoHeight(items []TodoItem, _ int) int {
	if len(items) == 0 {
		return 0
	}
	visible := len(items)
	if visible > 4 {
		visible = 4
	}
	rows := visible + 1
	if len(items) > visible {
		rows++
	}
	return rows
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func sanitizeBubbleText(text string) string {
	var builder strings.Builder
	for index := 0; index < len(text); {
		if text[index] == '\x1b' {
			index++
			if index < len(text) && text[index] == '[' {
				index++
				for index < len(text) {
					value := text[index]
					index++
					if value >= '@' && value <= '~' {
						break
					}
				}
			}
			continue
		}
		value, size := utf8.DecodeRuneInString(text[index:])
		index += size
		if value < 0x20 || value == 0x7f {
			builder.WriteByte(' ')
			continue
		}
		builder.WriteRune(value)
	}
	return builder.String()
}

type permissionRequestMsg struct {
	request permissionRequest
}

type permissionBridgeClosedMsg struct{}

type toolResultMsg struct {
	result tool.Result
	err    error
}

type turnResultMsg struct {
	events []applicationturn.Event
	result applicationturn.Result
	err    error
}
