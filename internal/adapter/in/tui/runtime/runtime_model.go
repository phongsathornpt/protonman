package runtime

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/agentui"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/conversation"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const (
	defaultBubbleWidth  = 80
	defaultBubbleHeight = 24
	promptRows          = 1
)

type agentLifecycleMsg struct{ event agent.Event }

type turnProgress struct {
	Round     int
	ToolCalls int
}

type bubbleModel struct {
	ctx                     context.Context
	service                 *toolcall.Service
	registry                tool.Registry
	skills                  *skill.Registry
	runner                  app.Conversation
	bridge                  *permissionBridge
	agents                  app.Agents
	agentEvents             <-chan agent.Event
	agentSnapshot           []agent.AgentStatus
	agentActivity           map[string]AgentActivity
	agentHistory            agentui.Tracker
	turnProgress            turnProgress
	activeTurnOwner         string
	workDir                 string
	viewport                viewport.Model
	transcriptViewport      viewport.Model
	spinner                 spinner.Model
	help                    help.Model
	keys                    bubbleKeyMap
	bottom                  *bottomPane
	historyState            *HistoryState
	queue                   []string
	todo                    []tododomain.Item
	todoStore               tododomain.Repository
	todoRevision            uint64
	todoLifecycle           todoLifecycleState
	busy                    bool
	activity                string
	pendingActivity         string
	planMode                bool
	conversationViewport    conversationViewportState
	showWelcome             bool
	showTranscript          bool
	rawTranscript           bool
	nextID                  uint64
	layout                  layoutState
	welcomeCache            welcomeCardCache
	busyStarted             time.Time
	turnCancel              context.CancelFunc
	turnEvents              <-chan tea.Msg
	messages                []model.Message
	conversationRetention   conversation.RetentionPolicy
	activeModel             string
	activeProvider          string
	providers               map[string]config.ProviderConfig
	maxToolCalls            int
	agentProfile            string
	subagentsEnabled        bool
	reasoningEffort         sdk.ReasoningEffort
	sessionID               string
	sessions                *app.Sessions
	workspaceKey            string
	modelCatalogs           modelcatalog.State
	runtimeConfig           config.RuntimeConfig
	projectTrusted          bool
	projectConfigSources    []string
	projectConfigProvenance map[string]config.ValueSource
	activeProviderSave      asyncOperationID
	activeProviderSelect    asyncOperationID
	activeProviderDelete    asyncOperationID
	activeModelSelect       asyncOperationID
}

type bubbleKeyMap struct {
	Submit       key.Binding
	Newline      key.Binding
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

func newBubbleModel(ctx context.Context, service *toolcall.Service, registry tool.Registry, todo []tododomain.Item, runner app.Conversation, bridge *permissionBridge, workDir string, initialMessages ...[]model.Message) *bubbleModel {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = brandStyle
	pane := viewport.New(viewport.WithWidth(defaultBubbleWidth), viewport.WithHeight(defaultBubbleHeight-6))
	disableViewportKeys(&pane)
	transcriptPane := viewport.New(viewport.WithWidth(defaultBubbleWidth-8), viewport.WithHeight(defaultBubbleHeight-8))
	disableViewportKeys(&transcriptPane)
	bottom := newBottomPane(runner != nil)
	helpView := help.New()
	helpView.SetWidth(defaultBubbleWidth - 2)
	helpView.ShortSeparator = glyphSep
	messages := []model.Message(nil)
	retention := conversation.DefaultRetentionPolicy()
	if len(initialMessages) > 0 {
		messages = conversation.Retain(model.SnapshotMessages(initialMessages[0]), retention)
	}
	ui := &bubbleModel{ctx: ctx, service: service, registry: registry, runner: runner, bridge: bridge, workDir: workDir, viewport: pane, transcriptViewport: transcriptPane, spinner: spin, help: helpView, keys: newBubbleKeyMap(), bottom: bottom, historyState: NewHistoryState(maxBubbleScrollback), queue: make([]string, 0), todo: append([]tododomain.Item{}, todo...), activity: "ready", conversationViewport: conversationViewportState{mode: viewportFollowing}, showWelcome: true, layout: layoutState{width: defaultBubbleWidth, height: defaultBubbleHeight}, messages: messages, conversationRetention: retention, maxToolCalls: config.DefaultMaxToolCalls, subagentsEnabled: true, runtimeConfig: config.DefaultRuntimeConfig(), agentActivity: make(map[string]AgentActivity)}
	if allTodoCompleted(ui.todo) {
		ui.todoLifecycle.CompletionFresh = true
	}
	ui.loadInitialMessages(messages)
	ui.syncPromptPlaceholder()
	ui.requestRelayout()
	ui.reconcileLayout()
	return ui
}

func disableViewportKeys(pane *viewport.Model) {
	// Keep page navigation owned by bubbles/viewport. Prompt-oriented arrows and
	// half-page bindings remain disabled so they cannot compete with textarea
	// cursor/history behavior.
	pane.KeyMap.HalfPageUp.SetEnabled(false)
	pane.KeyMap.HalfPageDown.SetEnabled(false)
	pane.KeyMap.Up.SetEnabled(false)
	pane.KeyMap.Down.SetEnabled(false)
	pane.KeyMap.Left.SetEnabled(false)
	pane.KeyMap.Right.SetEnabled(false)
}

func newBubbleKeyMap() bubbleKeyMap {
	return bubbleKeyMap{Submit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")), Newline: key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "newline")), Clear: key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "clear")), Quit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")), PageUp: key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "scroll")), PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "scroll")), ToggleTodo: key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "todos")), Transcript: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "transcript")), CycleMode: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "mode")), ToggleSkills: key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "skills")), ToggleModel: key.NewBinding(key.WithKeys("ctrl+p", "alt+m"), key.WithHelp("ctrl+p", "model"))}
}

func (k bubbleKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Newline, k.ToggleModel, k.Quit}
}

func (k bubbleKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Submit, k.Newline, k.Clear, k.Quit}, {k.PageUp, k.PageDown, k.ToggleTodo, k.Transcript}, {k.CycleMode, k.ToggleSkills, k.ToggleModel}}
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

func (m *bubbleModel) modelIDKnown(provider, modelID string) bool {
	if m == nil {
		return false
	}
	for _, candidate := range m.modelCatalogs.Models(provider) {
		if strings.EqualFold(strings.TrimSpace(candidate.ID), strings.TrimSpace(modelID)) {
			return true
		}
	}
	return false
}

func (m *bubbleModel) activeRemoteModel() (model.RemoteModel, bool) {
	if m == nil {
		return model.RemoteModel{}, false
	}
	for _, candidate := range m.modelCatalogs.Models(m.activeProvider) {
		if strings.EqualFold(strings.TrimSpace(candidate.ID), strings.TrimSpace(m.activeModel)) {
			return candidate, true
		}
	}
	return model.RemoteModel{}, false
}
