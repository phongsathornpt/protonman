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
	tuiconv "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/conversation"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionbridge"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/agentui"
	tuihistory "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/history"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/conversation"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const (
	defaultBubbleWidth      = 80
	defaultBubbleHeight     = 24
	promptRows              = 1
	composerNewlineEnhanced = "ctrl+enter"
	composerNewlineFallback = "ctrl+j"
)

type keyboardCapability uint8

const (
	keyboardCapabilityUnknown keyboardCapability = iota
	keyboardCapabilityLegacy
	keyboardCapabilityDisambiguated
)

func composerNewlineKeyNames() []string {
	return []string{composerNewlineEnhanced, composerNewlineFallback}
}

type agentLifecycleMsg struct{ event agent.Event }

type turnProgress struct {
	Round     int
	ToolCalls int
	Retry     sdk.RetryEvent
}

type reasoningPreferenceSource uint8

const (
	reasoningPreferenceConfig reasoningPreferenceSource = iota
	reasoningPreferenceSession
)

type agentModelState struct {
	agents                         app.Agents
	agentEvents                    <-chan agent.Event
	agentSnapshot                  []agent.AgentStatus
	agentActivity                  map[string]AgentActivity
	agentHistory                   agentui.Tracker
	agentProfile                   string
	subagentsEnabled               bool
	reasoningEffort                sdk.ReasoningEffort
	reasoningPreference            sdk.ReasoningEffort
	reasoningPreferenceSet         bool
	reasoningPreferenceSource      reasoningPreferenceSource
	reasoningCompatibilityFallback bool
}

type turnModelState struct {
	turnProgress    turnProgress
	activeTurnOwner string
	busy            bool
	activity        string
	pendingActivity string
	busyStarted     time.Time
	turnCancel      context.CancelFunc
	turnEvents      <-chan tea.Msg
}

type modelSetupState struct {
	activeModel          string
	activeProvider       string
	providers            map[string]config.ProviderConfig
	modelCatalogs        modelcatalog.State
	activeProviderSave   asyncOperationID
	activeProviderSelect asyncOperationID
	activeProviderDelete asyncOperationID
	activeModelSetup     asyncOperationID
	configMutationGate   *asyncOperationGate
}

type conversationModelState struct {
	historyState         *tuihistory.HistoryState
	conversationViewport conversationViewportState
	conversation         *tuiconv.State
	activeGoal           string
}

type todoModelState struct {
	todo               []tododomain.Item
	todoStore          tododomain.Repository
	todoHandlerFactory TodoHandlerFactory
	todoRevision       uint64
	todoLifecycle      todoLifecycleState
}

type sessionModelState struct {
	sessionID    string
	sessions     *app.Sessions
	workspaceKey string
}

type projectModelState struct {
	workDir                 string
	projectTrusted          bool
	projectConfigSources    []string
	projectConfigProvenance map[string]config.ValueSource
}

type executionPolicyState struct {
	maxToolCalls       int
	runtimeConfig      config.RuntimeConfig
	lowConcurrencyMode model.LowConcurrencySetting
}

type presentationModelState struct {
	viewport           viewport.Model
	spinner            spinner.Model
	help               help.Model
	keys               bubbleKeyMap
	planMode           bool
	reducedMotion      bool
	panes              paneState
	nextID             uint64
	layout             layoutState
	sessionHeaderCache sessionHeaderCache
	keyboardCapability keyboardCapability
	transientNotice    string
	transientNoticeID  uint64
}

type bubbleModel struct {
	ctx         context.Context
	service     *toolcall.Service
	registry    tool.Registry
	skills      *skill.Registry
	runner      app.Conversation
	application app.Services
	bridge      *permissionbridge.Bridge
	agentModelState
	turnModelState
	modelSetupState
	sessionModelState
	projectModelState
	conversationModelState
	todoModelState
	presentationModelState
	executionPolicyState
}

type bubbleKeyMap struct {
	Submit          key.Binding
	Newline         key.Binding
	Quit            key.Binding
	PageUp          key.Binding
	PageDown        key.Binding
	ToggleTodo      key.Binding
	Transcript      key.Binding
	CyclePermission key.Binding
	ToggleSkills    key.Binding
	ToggleModel     key.Binding
}

func newBubbleModel(ctx context.Context, service *toolcall.Service, registry tool.Registry, todo []tododomain.Item, runner app.Conversation, bridge *permissionbridge.Bridge, workDir string, initialMessages ...[]model.Message) *bubbleModel {
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = brandStyle
	reducedMotion := envconfig.Bool(envconfig.ReducedMotion)
	pane := viewport.New(viewport.WithWidth(defaultBubbleWidth), viewport.WithHeight(defaultBubbleHeight-6))
	disableViewportKeys(&pane)
	transcriptPane := viewport.New(viewport.WithWidth(defaultBubbleWidth-8), viewport.WithHeight(defaultBubbleHeight-8))
	disableViewportKeys(&transcriptPane)
	bottom := newBottomPane(runner != nil, reducedMotion)
	helpView := help.New()
	helpView.SetWidth(defaultBubbleWidth - 2)
	helpView.ShortSeparator = glyphSep
	retention := conversation.DefaultRetentionPolicy()
	ui := &bubbleModel{
		ctx:      ctx,
		service:  service,
		registry: registry,
		runner:   runner,
		bridge:   bridge,
		projectModelState: projectModelState{
			workDir: workDir,
		},
		presentationModelState: presentationModelState{
			viewport:      pane,
			spinner:       spin,
			help:          helpView,
			keys:          newBubbleKeyMap(),
			panes:         paneState{bottom: bottom, transcript: transcriptPane},
			reducedMotion: reducedMotion,
			layout:        layoutState{width: defaultBubbleWidth, height: defaultBubbleHeight},
		},
		conversationModelState: conversationModelState{
			historyState:         tuihistory.NewHistoryState(maxBubbleScrollback),
			conversationViewport: conversationViewportState{mode: viewportFollowing},
			conversation:         tuiconv.NewState(retention, initialMessages...),
		},
		todoModelState: todoModelState{
			todo: append([]tododomain.Item{}, todo...),
		},
		executionPolicyState: executionPolicyState{
			maxToolCalls:  runtimepolicy.TurnMaxToolCalls,
			runtimeConfig: config.DefaultRuntimeConfig(),
		},
		agentModelState: agentModelState{
			subagentsEnabled: true,
			agentActivity:    make(map[string]AgentActivity),
		},
		turnModelState:  turnModelState{activity: "ready"},
		modelSetupState: modelSetupState{configMutationGate: &asyncOperationGate{}},
	}

	if allTodoCompleted(ui.todo) {
		ui.todoLifecycle.CompletionFresh = true
	}
	ui.loadInitialMessages(ui.conversation.Messages())
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
	return bubbleKeyMap{Submit: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send message")), Newline: newComposerNewlineBinding(keyboardCapabilityUnknown), Quit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel or quit")), PageUp: key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "scroll")), PageDown: key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "scroll")), ToggleTodo: key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "tasks")), Transcript: key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "transcript")), CyclePermission: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "cycle permission")), ToggleSkills: key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "skills")), ToggleModel: key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "switch model"))}
}

func setComposerNewlineHelp(binding *key.Binding, capability keyboardCapability) {
	if binding == nil {
		return
	}
	if capability == keyboardCapabilityDisambiguated {
		binding.SetHelp(composerNewlineEnhanced, "new line")
		return
	}
	binding.SetHelp(composerNewlineFallback, "new line")
}

func (k bubbleKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Newline, k.ToggleModel, k.Quit}
}

func (k bubbleKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Submit, k.Newline, k.Quit}, {k.PageUp, k.PageDown, k.ToggleTodo, k.Transcript}, {k.CyclePermission, k.ToggleSkills, k.ToggleModel}}
}

func (m *bubbleModel) Init() tea.Cmd {
	commands := []tea.Cmd{m.bridge.Next()}
	// Reduced motion keeps the caret steady, so the blink loop never starts.
	if !m.reducedMotion {
		commands = append(commands, textarea.Blink)
	}
	commands = append(commands, m.nextAgentEvent())
	return tea.Batch(commands...)
}

// spinnerIndicator returns the animated busy frame, or an empty string when
// reduced motion is requested so callers fall back to the static brand mark.
func (m *bubbleModel) spinnerIndicator() string {
	if m == nil || m.reducedMotion {
		return ""
	}
	return m.spinner.View()
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
		clear(m.agentActivity)
		return
	}
	m.agentSnapshot = m.agents.List()
	if len(m.agentActivity) == 0 {
		return
	}
	retained := make(map[string]struct{}, len(m.agentSnapshot))
	for _, status := range m.agentSnapshot {
		retained[status.ID] = struct{}{}
	}
	for id := range m.agentActivity {
		if _, ok := retained[id]; !ok {
			delete(m.agentActivity, id)
		}
	}
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
