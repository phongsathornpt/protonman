package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/agentui"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	"log/slog"
	"runtime/debug"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultBubbleWidth  = 80
	defaultBubbleHeight = 24
	promptRows          = 1
)

var errTurnEventsClosed = errors.New("turn event stream closed before completion")

type agentLifecycleMsg struct{ event agent.Event }

type turnProgress struct {
	Round     int
	ToolCalls int
}

type bubbleModel struct {
	ctx                       context.Context
	service                   *toolcall.Service
	registry                  tool.Registry
	skills                    *skill.Registry
	runner                    app.Conversation
	bridge                    *permissionBridge
	agents                    app.Agents
	agentEvents               <-chan agent.Event
	agentSnapshot             []agent.AgentStatus
	agentActivity             map[string]AgentActivity
	agentHistory              agentui.Tracker
	turnProgress              turnProgress
	activeTurnOwner           string
	workDir                   string
	viewport                  viewport.Model
	transcriptViewport        viewport.Model
	spinner                   spinner.Model
	keys                      bubbleKeyMap
	bottom                    *bottomPane
	historyState              *HistoryState
	queue                     []string
	todo                      []TodoItem
	todoStore                 tododomain.Repository
	todoRevision              uint64
	todoViewState             todoViewState
	todoLifecycle             todoLifecycleState
	busy                      bool
	activity                  string
	pendingActivity           string
	planMode                  bool
	followTail                bool
	showWelcome               bool
	showTranscript            bool
	rawTranscript             bool
	viewportTailOnly          bool
	viewportStaleTail         bool
	viewportCommittedRevision uint64
	viewportActiveRevision    uint64
	viewportLineAnchors       []ScrollAnchor
	viewportViewCache         string
	viewportViewDirty         bool
	nextID                    uint64
	width                     int
	height                    int
	frameChrome               frameChrome
	welcomeCache              welcomeCardCache
	promptBoxCache            promptBoxChromeCache
	infoCache                 infoViewCache
	layoutGeneration          uint64
	busyStarted               time.Time
	turnCancel                context.CancelFunc
	turnEvents                <-chan tea.Msg
	messages                  []model.Message
	activeModel               string
	activeProvider            string
	providers                 map[string]config.ProviderConfig
	maxToolCalls              int
	agentProfile              string
	subagentsEnabled          bool
	reasoningEffort           sdk.ReasoningEffort
	sessionID                 string
	sessions                  *app.Sessions
	workspaceKey              string
	modelCatalogs             modelCatalogState
	runtimeConfig             config.RuntimeConfig
	projectTrusted            bool
	projectConfigSources      []string
	projectConfigProvenance   map[string]config.ValueSource
	blocks                    []Block // Compatibility snapshots for existing in-package tests during the
	// migration. Runtime ownership lives in bottom/historyState.

	prompt      *textarea.Model
	modal       *permissionRequest
	modalParked bool
	permIndex   int
	slashIndex  int
}

type bubbleKeyMap struct {
	Submit       key.Binding
	Clear        key.Binding
	Quit         key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	ToggleTodo   key.Binding
	Transcript   key.Binding
	CycleMode    key.Binding
	ToggleSkills key.Binding
	ToggleModel  key.Binding
}

func newBubbleModel(ctx context.Context, service *toolcall.Service, registry tool.Registry, todo []TodoItem, runner app.Conversation, bridge *permissionBridge, workDir string, initialMessages ...[]model.Message) *bubbleModel {
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
	ui := &bubbleModel{ctx: ctx, service: service, registry: registry, runner: runner, bridge: bridge, workDir: workDir, viewport: pane, transcriptViewport: transcriptPane, spinner: spin, keys: newBubbleKeyMap(), bottom: bottom, historyState: NewHistoryState(maxBubbleScrollback), queue: make([]string, 0), todo: append([]TodoItem{}, todo...), activity: "ready", followTail: true, showWelcome: true, width: defaultBubbleWidth, height: defaultBubbleHeight, messages: messages, maxToolCalls: config.DefaultMaxToolCalls, subagentsEnabled: true, runtimeConfig: config.DefaultRuntimeConfig(), agentActivity: make(map[string]AgentActivity)}
	if allTodoCompleted(ui.todo) {
		ui.todoLifecycle.CompletionFresh = true
	}
	ui.prompt = bottom.prompt()
	ui.loadInitialMessages(messages)
	ui.syncComponentsToLegacy()
	ui.syncPromptPlaceholder()
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
	return bubbleKeyMap{Submit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")), Clear: key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "clear")), Quit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")), PageUp: key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "scroll")), PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "scroll")), ToggleTodo: key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "todos")), Transcript: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "transcript")), CycleMode: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "mode")), ToggleSkills: key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "skills")), ToggleModel: key.NewBinding(key.WithKeys("ctrl+p", "alt+m"), key.WithHelp("ctrl+p", "model"))}
}

func (m *bubbleModel) Init() tea.Cmd {
	return tea.Batch(m.bridge.Next(), textarea.Blink, m.nextAgentEvent())
}

func (m *bubbleModel) nextAgentEvent() tea.Cmd {
	if m.agentEvents == nil {
		return nil
	}
	events := m.agentEvents
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return nil
		}
		return agentLifecycleMsg{event: ev}
	}
}

func (m *bubbleModel) syncAgentSnapshot() {
	if !m.agents.Available() {
		m.agentSnapshot = nil
		return
	}
	m.agentSnapshot = m.agents.List()
}

const (
	maxQueuedPrompts     = 32
	maxQueuePreviewRunes = 160
)

func (m *bubbleModel) submit() tea.Cmd {
	m.syncLegacyToComponents()
	prompt := m.bottom.prompt()
	line := strings.TrimSpace(prompt.Value())
	if m.bottom.bashMode() {
		if line == "" {
			m.resetPrompt()
			m.bottom.remove(slashViewID)
			m.setBashMode(false)
			return nil
		}
		if m.busy || m.hasPermissionView() {
			if !m.enqueuePrompt("!" + line) {
				return nil
			}
			m.resetPrompt()
			m.bottom.remove(slashViewID)
			m.refreshViewport()
			return nil
		}
		m.resetPrompt()
		m.bottom.remove(slashViewID)
		m.setBashMode(false)
		return m.dispatchBang(line)
	}
	if line == "" {
		return nil
	}
	if m.busy || m.hasPermissionView() {
		if !m.enqueuePrompt(line) {
			return nil
		}
		m.resetPrompt()
		m.bottom.remove(slashViewID)
		m.refreshViewport()
		return nil
	}
	m.resetPrompt()
	m.bottom.remove(slashViewID)
	return m.dispatch(line)
}

func (m *bubbleModel) enqueuePrompt(line string) bool {
	if len(m.queue) >= maxQueuedPrompts {
		m.appendMuted(fmt.Sprintf("queue full (%d); finish or cancel the active turn before adding more", maxQueuedPrompts))
		m.refreshViewport()
		return false
	}
	m.queue = append(m.queue, line)
	m.appendMuted(fmt.Sprintf("queued (%d): %s", len(m.queue), queuePreview(line)))
	return true
}

func queuePreview(line string) string {
	runes := []rune(strings.TrimSpace(line))
	if len(runes) <= maxQueuePreviewRunes {
		return string(runes)
	}
	return string(runes[:maxQueuePreviewRunes-1]) + "…"
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
	slog.DebugContext(m.ctx, "tui direct bash submitted", "command_bytes", len(command))
	m.bottom.recordHistory("!" + command)
	m.appendUser("!" + command)
	return m.startBash(command)
}

func (m *bubbleModel) startTool(call tool.Call) tea.Cmd {
	slog.DebugContext(m.ctx, "tui direct tool started", "call_id", call.ID, "tool_name", call.Name, "argument_bytes", len(call.Arguments))
	m.messages = append(m.messages, model.Message{Role: model.RoleAssistant, ToolCalls: []model.ToolCall{{ID: call.ID, Name: call.Name, Arguments: append([]byte(nil), call.Arguments...)}}})
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
		startedAt := time.Now()
		result, callErr := m.service.Call(ctx, call)
		attrs := []any{"call_id", call.ID, "tool_name", call.Name, "duration_ms", time.Since(startedAt).Milliseconds(), "success", callErr == nil, "error_type", errorType(callErr)}
		if result.Failure != nil {
			attrs = append(attrs, "error_code", result.Failure.Code)
		}
		slog.DebugContext(ctx, "tui direct tool worker returned", attrs...)
		return toolResultMsg{call: call, result: result, err: callErr}
	}
}

func (m *bubbleModel) appendModelToolResult(call tool.Call, result tool.Result) {
	if result.CallID == "" {
		result.CallID = call.ID
	}
	if result.ToolName == "" {
		result.ToolName = call.Name
	}
	content, err := json.Marshal(result)
	if err != nil {
		content = []byte(fmt.Sprintf(`{"call_id":%q,"tool_name":%q,"error":{"code":"execution_error","message":%q}}`, call.ID, call.Name, err.Error()))
	}
	m.messages = append(m.messages, model.Message{Role: model.RoleTool, Content: string(content), ToolCallID: result.CallID, ToolName: result.ToolName})
}

func (m *bubbleModel) reconfigureRunner() {
	if m.activeModel == "" || m.service == nil {
		return
	}
	provName := m.activeProvider
	if provName == "" {
		provName = model.DefaultProtonmanName
	}
	prov, ok := m.providers[strings.ToLower(provName)]
	hasValidAuth := ok && model.ProviderHasUsableAuth(provName, prov.BaseURL, prov.APIKey)
	if !hasValidAuth {
		loaded, err := (app.Providers{}).LoadConfigured(m.ctx, m.workDir)
		if err == nil {
			if m.providers == nil {
				m.providers = make(map[string]config.ProviderConfig)
			}
			for key, value := range loaded {
				m.providers[key] = value
			}
			prov, ok = m.providers[strings.ToLower(provName)]
			hasValidAuth = ok && model.ProviderHasUsableAuth(provName, prov.BaseURL, prov.APIKey)
		}
	}
	if !hasValidAuth {
		m.runner = nil
		m.syncPromptPlaceholder()
		return
	}
	sessID := m.sessionID
	if sessID == "" && m.workDir != "" {
		sessID = "workspace-" + m.workDir
	}
	var remote *model.RemoteModel
	if resolved, ok := m.activeRemoteModel(); ok {
		remote = &resolved
	}
	conversation, err := app.BuildConversation(m.service, m.skills, m.agents, app.ConversationSpec{ProviderName: provName, ProviderType: prov.Type, BaseURL: prov.BaseURL, APIKey: prov.APIKey, ModelID: m.activeModel, SessionID: sessID, Workspace: m.workDir, AgentProfile: m.agentProfile, ReasoningEffort: m.reasoningEffort, MaxToolCalls: m.maxToolCalls, RequestTimeout: m.runtimeConfig.ModelRequestTimeout, TurnTimeout: m.runtimeConfig.TurnTimeout, RoundTimeout: m.runtimeConfig.RoundTimeout, RemoteModel: remote})
	if err != nil {
		m.appendError("failed to configure model runner: " + err.Error())
		m.runner = nil
		m.syncPromptPlaceholder()
		return
	}
	if conversation != nil {
		m.runner = conversation
		m.syncPromptPlaceholder()
	}
}

func (m *bubbleModel) setPermissionMode(mode permission.Mode) error {
	if err := m.service.SetMode(mode); err != nil {
		return err
	}
	m.agents.SetPermissionMode(mode)
	m.syncPromptPlaceholder()
	return nil
}

func (m *bubbleModel) syncPromptPlaceholder() {
	if m == nil || m.bottom == nil {
		return
	}
	mode := permission.ModeAsk
	if m.service != nil {
		mode = m.service.Mode()
	}
	hasRunner := m.runner != nil
	m.bottom.setPlaceholder(promptPlaceholder(hasRunner, mode, m.planMode))
}

var tuiTurnOwnerSeq atomic.Uint64

func (m *bubbleModel) startTurn(prompt string) tea.Cmd {
	if m.runner == nil {
		m.reconfigureRunner()
	}
	if m.runner == nil {
		slog.DebugContext(m.ctx, "tui turn rejected", "reason", "runner_unavailable")
		m.appendError("model client is not configured; use /model or /provider add to configure")
		m.relayout()
		return nil
	}
	m.retireCompletedTodoForNextTurn()
	m.messages = append(m.messages, model.Message{Role: model.RoleUser, Content: prompt})
	m.busy = true
	m.busyStarted = time.Now()
	m.turnProgress = turnProgress{}
	m.activeTurnOwner = fmt.Sprintf("tui-turn-%d", tuiTurnOwnerSeq.Add(1))
	m.activity = "analyzing"
	m.historyState.SetSpinnerFrame(m.spinner.View())
	m.historyState.StartThinking()
	m.relayout()
	ctx, cancel := context.WithCancel(m.ctx)
	ctx = agent.WithTurnRef(ctx, agent.TurnRef{SessionID: m.sessionID, TurnID: m.activeTurnOwner})
	m.turnCancel = cancel
	events := make(chan tea.Msg, 32)
	history := model.CloneMessages(m.messages)
	startedAt := time.Now()
	slog.DebugContext(ctx, "tui turn started", "prompt_bytes", len(prompt), "history_messages", len(history))
	go func() {
		queueTerminal := func(result app.Result, err error) {
			select {
			case events <- turnDoneMsg{result: result, err: err}:
				slog.DebugContext(ctx, "tui turn terminal message queued")
			case <-m.ctx.Done():
				slog.DebugContext(ctx, "tui turn terminal message dropped", "reason", "ui_context_done")
			}
		}
		defer func() {
			if panicValue := recover(); panicValue != nil {
				stack := debug.Stack()
				slog.DebugContext(ctx, "tui turn worker panicked", "panic_type", fmt.Sprintf("%T", panicValue), "stack_bytes", len(stack))
				queueTerminal(app.Result{}, fmt.Errorf("turn worker panicked: %v", panicValue))
			}
			close(events)
			slog.DebugContext(ctx, "tui turn event channel closed", "duration_ms", time.Since(startedAt).Milliseconds())
		}()
		result, err := m.runner.Run(ctx, history, func(runCtx context.Context, event app.Event) error {
			select {
			case events <- turnDeltaMsg{event: event}:
				return nil
			case <-runCtx.Done():
				return runCtx.Err()
			}
		})
		slog.DebugContext(ctx, "tui turn runner returned", "duration_ms", time.Since(startedAt).Milliseconds(), "success", err == nil, "error_type", errorType(err), "rounds", result.Rounds, "message_count", len(result.Messages))
		queueTerminal(result, err)
	}()
	m.turnEvents = events
	return waitTurnCh(events)
}

func (m *bubbleModel) withSpinner(command tea.Cmd) tea.Cmd {
	if command == nil || !m.busy {
		return command
	}
	return tea.Batch(m.spinner.Tick, command)
}

func waitTurnCh(events <-chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			slog.Debug("tui turn wait observed closed event channel")
			return turnEventsClosedMsg{}
		}
		return msg
	}
}

func errorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}

func (m *bubbleModel) cancelActiveTurn() int {
	if !m.busy || m.turnCancel == nil {
		return 0
	}
	m.activity = "canceling"
	stopping := 0
	if m.agents.Available() && m.activeTurnOwner != "" {
		stopping = m.agents.CancelTurn(m.activeTurnOwner, agent.CancelTurnAndChildren)
		m.syncAgentSnapshot()
	}
	m.turnCancel()
	m.relayout()
	return stopping
}

func (m *bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.syncLegacyToComponents()
	defer m.syncComponentsToLegacy()
	switch message := msg.(type) {
	case agentLifecycleMsg:
		return m.updateAgentLifecycle(message)
	case tea.WindowSizeMsg:
		m.resize(message.Width, message.Height)
		return m, nil
	case tea.KeyMsg:
		if key.Matches(message, m.keys.Quit) {
			return m.handleInterruptKey()
		}
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
		if m.bottom.has(skillsViewID) {
			if view, ok := m.bottom.find(skillsViewID).(*skillsPaneView); ok {
				switch message.Button {
				case tea.MouseButtonWheelUp:
					view.HandleKey(m, tea.KeyMsg{Type: tea.KeyUp})
					m.relayout()
					return m, nil
				case tea.MouseButtonWheelDown:
					view.HandleKey(m, tea.KeyMsg{Type: tea.KeyDown})
					m.relayout()
					return m, nil
				}
			}
		}
		if message.Y < 0 || message.Y >= m.viewport.Height {
			return m, nil
		}
		if (m.viewportTailOnly || m.viewportStaleTail) && (message.Button == tea.MouseButtonWheelUp || message.Button == tea.MouseButtonWheelDown) {
			m.hydrateViewportForScroll()
		}
		beforeOffset := m.viewport.YOffset
		m.viewport, command = m.viewport.Update(message)
		if m.viewport.YOffset != beforeOffset {
			m.markViewportViewDirty()
		}
		m.followTail = m.viewport.AtBottom()
		return m, command
	case spinner.TickMsg:
		var command tea.Cmd
		m.spinner, command = m.spinner.Update(message)
		if !m.busy {
			return m, nil
		}
		if m.historyState.SetSpinnerFrame(m.spinner.View()) {
			m.refreshViewport()
		}
		return m, command
	case cursor.BlinkMsg:
		prompt := m.bottom.prompt()
		updated, command := prompt.Update(message)
		*prompt = updated
		return m, command
	case permissionRequestMsg:
		return m.updatePermissionRequest(message)
	case permissionBridgeClosedMsg:
		return m, nil
	case toolResultMsg:
		return m.updateToolResult(message)
	case modelsFetchedMsg:
		return m.updateModelsFetched(message)
	case providerSavedMsg:
		return m.updateProviderSaved(message)
	case modelSelectedMsg:
		return m.updateModelSelected(message)
	case providerActiveSelectedMsg:
		return m.updateProviderActiveSelected(message)
	case providerDeletedMsg:
		return m.updateProviderDeleted(message)
	case projectInitializedMsg:
		return m.updateProjectInitialized(message)
	case projectSettingSavedMsg:
		return m.updateProjectSettingSaved(message)
	case userSettingSavedMsg:
		return m.updateUserSettingSaved(message)
	case permissionRuleSavedMsg:
		return m.updatePermissionRuleSaved(message)
	case projectLoadedMsg:
		return m.updateProjectLoaded(message)
	case turnDeltaMsg:
		return m.updateTurnDelta(message)
	case turnEventsClosedMsg:
		return m.updateTurnEventsClosed(message)
	case turnDoneMsg:
		return m.updateTurnDone(message)
	}
	return m, nil
}

func (m *bubbleModel) matchesGlobalShortcut(message tea.KeyMsg) bool {
	return key.Matches(message, m.keys.Clear) || key.Matches(message, m.keys.ToggleTodo) || key.Matches(message, m.keys.Transcript) || key.Matches(message, m.keys.CycleMode) || key.Matches(message, m.keys.ToggleSkills) || key.Matches(message, m.keys.ToggleModel)
}

func (m *bubbleModel) handleInterruptKey() (tea.Model, tea.Cmd) {
	if m.showTranscript {
		m.showTranscript = false
		m.relayout()
		return m, nil
	}
	if top := m.bottom.top(); top != nil && top.ID() != permissionViewID && top.ID() != slashViewID {
		if provider, ok := top.(*providerPaneView); ok {
			provider.cancelFetch()
		}
		m.bottom.remove(top.ID())
		m.relayout()
		return m, nil
	}
	if m.busy && m.turnCancel != nil {
		m.cancelActiveTurn()
		m.queue = nil
		return m, nil
	}
	prompt := m.bottom.prompt()
	if prompt.Value() != "" || m.bottom.bashMode() {
		m.resetPrompt()
		m.setBashMode(false)
		m.syncSlashView()
		m.relayout()
		return m, nil
	}
	return m, tea.Quit
}

func (m *bubbleModel) updateKey(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if handled, command := m.handleModalKey(message); handled {
		return m, m.withSpinner(command)
	}
	if handled, command := m.handleGlobalKey(message); handled {
		return m, m.withSpinner(command)
	}
	return m, m.handlePromptKey(message)
}

func (m *bubbleModel) handleModalKey(message tea.KeyMsg) (bool, tea.Cmd) {
	top := m.bottom.top()
	if top == nil {
		return false, nil
	}
	handled, command := top.HandleKey(m, message)
	if handled {
		m.relayout()
	}
	return handled, command
}

func (m *bubbleModel) handleGlobalKey(message tea.KeyMsg) (bool, tea.Cmd) {
	switch {
	case key.Matches(message, m.keys.CycleMode):
		m.cycleMode()
		return true, nil
	case key.Matches(message, m.keys.Transcript):
		m.showTranscript = true
		m.refreshTranscriptViewport(true)
		return true, nil
	case key.Matches(message, m.keys.ToggleSkills):
		if m.bottom.has(skillsViewID) {
			m.bottom.remove(skillsViewID)
			m.relayout()
			return true, nil
		}
		if m.skills != nil && len(m.skills.List()) > 0 {
			m.bottom.push(&skillsPaneView{})
			m.relayout()
			return true, nil
		}
		m.executeCommand("/skills")
		return true, nil
	case key.Matches(message, m.keys.ToggleModel):
		if m.bottom.has(modelSelectViewID) {
			m.bottom.remove(modelSelectViewID)
			m.relayout()
			return true, nil
		}
		return true, m.openModelSelectPane()
	case key.Matches(message, m.keys.Clear):
		m.resetTranscript()
		m.refreshViewport()
		return true, nil
	case key.Matches(message, m.keys.ToggleTodo):
		if layoutModeForHeight(m.height) != layoutNormal {
			m.toggleTodoPane()
			return true, nil
		}
		m.todoViewState.Expanded = !m.todoViewState.Expanded
		if m.todoViewState.Expanded {
			m.revealRetiredTodo()
		}
		m.relayout()
		return true, nil
	case key.Matches(message, m.keys.PageUp):
		m.hydrateViewportForScroll()
		before := m.viewport.YOffset
		m.viewport.PageUp()
		if m.viewport.YOffset != before {
			m.markViewportViewDirty()
		}
		m.followTail = m.viewport.AtBottom()
		return true, nil
	case key.Matches(message, m.keys.PageDown):
		m.hydrateViewportForScroll()
		before := m.viewport.YOffset
		m.viewport.PageDown()
		if m.viewport.YOffset != before {
			m.markViewportViewDirty()
		}
		m.followTail = m.viewport.AtBottom()
		return true, nil
	default:
		return false, nil
	}
}

func (m *bubbleModel) handlePromptKey(message tea.KeyMsg) tea.Cmd {
	if message.String() == "tab" && m.busy {
		return m.withSpinner(m.submit())
	}
	prompt := m.bottom.prompt()
	if message.String() == "esc" {
		if m.bottom.bashMode() {
			m.setBashMode(false)
		}
		m.resetPrompt()
		m.syncSlashView()
		m.relayout()
		return nil
	}
	if message.String() == "enter" {
		return m.withSpinner(m.submit())
	}
	if !m.bottom.bashMode() && prompt.Value() == "" && message.String() == "!" {
		m.setBashMode(true)
		return nil
	}
	if m.bottom.bashMode() && prompt.Value() == "" {
		switch message.Type {
		case tea.KeyBackspace, tea.KeyCtrlH, tea.KeyDelete:
			m.setBashMode(false)
			return nil
		}
	}
	if message.String() == "up" {
		lineInfo := prompt.LineInfo()
		if prompt.LineCount() == 1 || (prompt.Line() == 0 && lineInfo.RowOffset == 0 && lineInfo.ColumnOffset == 0) {
			m.historyPrevious()
			m.syncSlashView()
			return nil
		}
	}
	if message.String() == "down" && m.bottom.historyNavigating() {
		m.historyNext()
		m.syncSlashView()
		return nil
	}
	updated, command := prompt.Update(message)
	*prompt = updated
	m.syncSlashView()
	m.relayout()
	return command
}

func (m *bubbleModel) updateAgentLifecycle(message agentLifecycleMsg) (tea.Model, tea.Cmd) {
	if m.agentActivity == nil {
		m.agentActivity = make(map[string]AgentActivity)
	}
	if message.event.Kind == agent.EventAgentProgress {
		activity := agentActivityFromEvent(message.event)
		if activity.String() != "" {
			m.agentActivity[message.event.AgentID] = activity
			if run := m.ensureHistoryState().AgentRun(message.event.AgentID); run != nil {
				run.Activity = activity.String()
				m.ensureHistoryState().TouchAgentRun(message.event.AgentID)
			}
			m.relayout()
		}
		return m, m.nextAgentEvent()
	}
	if message.event.Kind == agent.EventAgentCompleted || message.event.Kind == agent.EventAgentFailed {
		delete(m.agentActivity, message.event.AgentID)
	}
	m.syncAgentSnapshot()
	m.syncAgentRunSnapshot(message.event.AgentID)
	m.relayout()
	return m, m.nextAgentEvent()
}

func (m *bubbleModel) updatePermissionRequest(message permissionRequestMsg) (tea.Model, tea.Cmd) {
	if !m.busy {
		message.request.response <- permissionResponse{resolution: permission.Resolution{Action: permission.ActionDeny, Reason: "turn is no longer active"}, err: context.Canceled}
		return m, m.bridge.Next()
	}
	m.openPermission(message.request)
	m.relayout()
	return m, m.bridge.Next()
}

func (m *bubbleModel) updateToolResult(message toolResultMsg) (tea.Model, tea.Cmd) {
	slog.DebugContext(m.ctx, "tui direct tool completed", "call_id", message.call.ID, "tool_name", message.call.Name, "success", message.err == nil, "error_type", errorType(message.err))
	m.busy = false
	m.busyStarted = time.Time{}
	m.turnProgress = turnProgress{}
	m.activity = "ready"
	m.turnCancel = nil
	m.appendToolResult(message.result, message.err)
	m.syncTodoSnapshot()
	if message.call.ID != "" {
		m.appendModelToolResult(message.call, message.result)
	}
	m.relayout()
	return m, m.withSpinner(m.drainQueue())
}

func (m *bubbleModel) updateModelsFetched(message modelsFetchedMsg) (tea.Model, tea.Cmd) {
	if pane := m.bottom.find(providerViewID); pane != nil {
		if pv, ok := pane.(*providerPaneView); ok {
			if pv.fetchRequestID != 0 && message.requestID != pv.fetchRequestID {
				return m, nil
			}
			pv.fetchCancel = nil
			if message.err == nil && len(message.models) > 0 {
				m.modelCatalogs.set(message.providerName, message.models)
			}
			if message.err != nil {
				pv.state = providerStateError
				pv.errorMessage = message.err.Error()
			} else {
				pv.state = providerStateSelectModel
				pv.setFetchedModels(message.models)
			}
			m.relayout()
		}
		return m, nil
	}
	if pane := m.bottom.find(modelSelectViewID); pane != nil {
		if mv, ok := pane.(*modelSelectPaneView); ok {
			currentProvider := mv.activeProviderName()
			if message.requestID != mv.fetchRequestID || !strings.EqualFold(message.providerName, currentProvider) {
				return m, nil
			}
			mv.fetchCancel = nil
			mv.loading = false
			mv.err = message.err
			if message.err == nil {
				m.modelCatalogs.set(message.providerName, message.models)
				mv.setModels(m.modelCatalogs.models(message.providerName), m.activeModel)
			}
			m.relayout()
		}
	}
	return m, nil
}

func (m *bubbleModel) updateProviderSaved(message providerSavedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		if pane := m.bottom.find(providerViewID); pane != nil {
			if pv, ok := pane.(*providerPaneView); ok {
				pv.state = providerStateSaveError
				pv.errorMessage = message.err.Error()
				m.relayout()
				return m, nil
			}
		}
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to save provider: %v", message.err)))
	} else {
		providerName := strings.TrimSpace(message.providerName)
		providerKey := strings.ToLower(providerName)
		previousKey := strings.ToLower(strings.TrimSpace(message.previousName))
		if previousKey != "" && previousKey != providerKey {
			delete(m.providers, previousKey)
		}
		if m.providers == nil {
			m.providers = make(map[string]config.ProviderConfig)
		}
		m.providers[providerKey] = config.ProviderConfig{Name: providerName, Type: message.providerType, BaseURL: message.baseURL, APIKey: message.apiKey}
		if message.activated {
			m.activeModel = message.modelID
			m.activeProvider = providerName
			m.reconfigureRunner()
			m.appendLine(successStyle.Render(fmt.Sprintf("✓ Configured provider %s", providerName)))
		} else {
			m.appendLine(successStyle.Render(fmt.Sprintf("✓ Updated provider %s", providerName)))
			if m.activeProvider != "" {
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Active provider remains %s", m.activeProvider)))
			}
		}
		m.appendLine(mutedStyle.Render(fmt.Sprintf("  Endpoint: %s", message.baseURL)))
		if message.activated && message.modelID != "" {
			m.appendLine(mutedStyle.Render(fmt.Sprintf("  Default Model: %s", message.modelID)))
		}
		m.appendLine(mutedStyle.Render("  Saved to " + appdirs.UserConfigDisplay()))
	}
	m.bottom.remove(providerViewID)
	m.relayout()
	return m, nil
}

func (m *bubbleModel) updateModelSelected(message modelSelectedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to set active model: %v", message.err)))
	} else {
		m.activeModel = message.modelID
		if message.providerName != "" {
			m.activeProvider = message.providerName
		}
		if m.reasoningEffort != sdk.ReasoningDefault {
			profile := m.activeResolvedModelProfile()
			if _, err := profile.ResolveExplicitReasoning(m.reasoningEffort); err != nil {
				previous := m.reasoningEffort
				m.reasoningEffort = sdk.ReasoningDefault
				m.agents.SetReasoningEffort(sdk.ReasoningDefault)
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Reset thinking level to auto (previous level %q is unsupported by %s)", previous, message.modelID)))
			}
		}
		m.reconfigureRunner()
		m.appendLine(successStyle.Render(fmt.Sprintf("✓ Active model set to %s (%s)", message.modelID, m.activeProvider)))
		if message.unverified {
			m.appendLine(mutedStyle.Render("  Model ID was not present in the discovered catalog; using it as a custom model."))
		}
		m.appendLine(mutedStyle.Render("  Saved to " + appdirs.UserConfigDisplay()))
	}
	m.bottom.remove(modelSelectViewID)
	m.relayout()
	return m, nil
}

func (m *bubbleModel) updateProviderActiveSelected(message providerActiveSelectedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to switch provider: %v", message.err)))
	} else {
		m.activeProvider = message.providerName
		models := m.modelCatalogs.models(message.providerName)
		if len(models) > 0 {
			found := false
			for _, mod := range models {
				if strings.EqualFold(mod.ID, m.activeModel) {
					found = true
					break
				}
			}
			if !found {
				targetModel := models[0].ID
				m.activeModel = targetModel
				_ = (app.Providers{}).SelectModel(message.providerName, targetModel)
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Reconciled active model to %s", targetModel)))
			}
		}
		if m.reasoningEffort != sdk.ReasoningDefault {
			profile := m.activeResolvedModelProfile()
			if _, err := profile.ResolveExplicitReasoning(m.reasoningEffort); err != nil {
				previous := m.reasoningEffort
				m.reasoningEffort = sdk.ReasoningDefault
				m.agents.SetReasoningEffort(sdk.ReasoningDefault)
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Reset thinking level to auto (previous level %q is unsupported by %s)", previous, m.activeModel)))
			}
		}
		m.reconfigureRunner()
		m.appendLine(successStyle.Render(fmt.Sprintf("✓ Switched active provider to %s", message.providerName)))
		if p, ok := m.providers[strings.ToLower(message.providerName)]; ok && p.BaseURL != "" {
			m.appendLine(mutedStyle.Render(fmt.Sprintf("  Endpoint: %s", p.BaseURL)))
		}
		if m.activeModel != "" {
			m.appendLine(mutedStyle.Render(fmt.Sprintf("  Active model: %s", m.activeModel)))
		} else {
			m.appendLine(mutedStyle.Render("  Use /model to choose a model for this provider"))
		}
		m.appendLine(mutedStyle.Render("  Saved to " + appdirs.UserConfigDisplay()))
	}
	m.bottom.remove(providerSelectViewID)
	m.relayout()
	return m, nil
}

func (m *bubbleModel) updateProviderDeleted(message providerDeletedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to remove provider %s: %v", message.providerName, message.err)))
	} else {
		delete(m.providers, strings.ToLower(message.providerName))
		if strings.EqualFold(m.activeProvider, message.providerName) {
			m.activeProvider = ""
			if len(m.providers) > 0 {
				keys := make([]string, 0, len(m.providers))
				for k := range m.providers {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				m.activeProvider = keys[0]
				models := m.modelCatalogs.models(m.activeProvider)
				if len(models) > 0 {
					m.activeModel = models[0].ID
				} else {
					m.activeModel = ""
				}
				m.reconfigureRunner()
			} else {
				m.activeModel = ""
				m.runner = nil
				m.bottom.setHasRunner(false)
			}
		}
		m.appendLine(successStyle.Render(fmt.Sprintf("✓ Removed provider %s", message.providerName)))
		if m.activeProvider != "" {
			m.appendLine(mutedStyle.Render(fmt.Sprintf("  Active provider is now %s", m.activeProvider)))
		}
		m.appendLine(mutedStyle.Render("  Updated " + appdirs.UserConfigDisplay()))
	}
	m.bottom.remove(providerSelectViewID)
	m.relayout()
	return m, nil
}

func (m *bubbleModel) updateTurnDelta(message turnDeltaMsg) (tea.Model, tea.Cmd) {
	batch := []app.Event{message.event}
	for {
		select {
		case next, ok := <-m.turnEvents:
			if !ok {
				m.applyTurnEvents(batch)
				slog.DebugContext(m.ctx, "tui turn event channel closed before terminal message", "busy", m.busy)
				if m.busy && m.ctx.Err() == nil {
					return m.Update(turnEventsClosedMsg{})
				}
				m.refreshViewport()
				return m, nil
			}
			if delta, isDelta := next.(turnDeltaMsg); isDelta {
				batch = append(batch, delta.event)
				continue
			}
			m.applyTurnEvents(batch)
			// The terminal message performs its own relayout/viewport refresh.
			// Avoid rendering the just-drained deltas twice at turn completion.
			return m.Update(next)
		default:
		}
		break
	}
	m.applyTurnEvents(batch)
	m.refreshViewport()
	return m, m.withSpinner(waitTurnCh(m.turnEvents))
}

func (m *bubbleModel) updateTurnEventsClosed(message turnEventsClosedMsg) (tea.Model, tea.Cmd) {
	slog.DebugContext(m.ctx, "tui turn event channel closed unexpectedly", "busy", m.busy, "context_error", m.ctx.Err() != nil)
	if !m.busy || m.ctx.Err() != nil {
		return m, nil
	}
	return m.Update(turnDoneMsg{err: errTurnEventsClosed})
}

func (m *bubbleModel) updateTurnDone(message turnDoneMsg) (tea.Model, tea.Cmd) {
	slog.DebugContext(m.ctx, "tui turn terminal message received", "success", message.err == nil, "error_type", errorType(message.err), "rounds", message.result.Rounds, "message_count", len(message.result.Messages), "assistant_bytes", len(message.result.Message.Content))
	m.busy = false
	m.busyStarted = time.Time{}
	m.activity = "ready"
	m.turnCancel = nil
	m.turnEvents = nil
	m.activeTurnOwner = ""
	if message.err != nil {
		m.finalizeRunningTools(message.err)
	}
	m.historyState.CommitActive()
	m.syncLegacyBlocks()
	if message.err == nil {
		if len(message.result.Messages) > 0 {
			m.messages = append(m.messages, model.CloneMessages(message.result.Messages)...)
		} else if message.result.Message.Content != "" {
			m.messages = append(m.messages, message.result.Message)
		}
	} else if message.err != nil && len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == model.RoleUser {
		m.messages = m.messages[:len(m.messages)-1]
	}
	m.appendTurnFailure(message.err)
	m.relayout()
	if message.err != nil {
		m.queue = nil
		return m, nil
	}
	return m, m.withSpinner(m.drainQueue())
}
