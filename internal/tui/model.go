package tui

import (
	"context"
	"errors"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/skill"
	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
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
	ctx           context.Context
	service       *toolcall.Service
	registry      tool.Registry
	skills        *skill.Registry
	runner        applicationturn.Runner
	bridge        *permissionBridge
	coordinator   *agent.Coordinator
	agentEvents   <-chan agent.Event
	agentSnapshot []agent.AgentStatus
	agentActivity map[string]string
	turnProgress  turnProgress
	workDir       string

	viewport           viewport.Model
	transcriptViewport viewport.Model
	spinner            spinner.Model
	keys               bubbleKeyMap
	bottom             *bottomPane
	historyState       *HistoryState

	queue            []string
	todo             []TodoItem
	todoStore        tododomain.Repository
	todoRevision     uint64
	todoExpanded     bool
	busy             bool
	activity         string
	pendingActivity  string
	planMode         bool
	followTail       bool
	showWelcome      bool
	showTranscript   bool
	rawTranscript    bool
	viewportTailOnly bool
	nextID           uint64
	width            int
	height           int
	busyStarted      time.Time
	turnCancel       context.CancelFunc
	turnEvents       <-chan tea.Msg
	messages         []model.Message
	activeModel      string
	activeProvider   string
	providers        map[string]config.ProviderConfig
	maxRounds        int
	maxToolCalls     int
	agentProfile     string
	sessionID        string
	modelCatalogs    modelCatalogState
	runtimeConfig    config.RuntimeConfig

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
		maxRounds:          config.DefaultMaxRounds,
		maxToolCalls:       config.DefaultMaxToolCalls,
		runtimeConfig:      config.DefaultRuntimeConfig(),
		agentActivity:      make(map[string]string),
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
		Submit:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
		Clear:        key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "clear")),
		Quit:         key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		PageUp:       key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "scroll")),
		PageDown:     key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "scroll")),
		ToggleTodo:   key.NewBinding(key.WithKeys("ctrl+o"), key.WithHelp("ctrl+o", "todos")),
		Transcript:   key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "transcript")),
		CycleMode:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "mode")),
		ToggleSkills: key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "skills")),
		ToggleModel:  key.NewBinding(key.WithKeys("ctrl+p", "alt+m"), key.WithHelp("ctrl+p", "model")),
	}
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
	if m.coordinator == nil {
		m.agentSnapshot = nil
		return
	}
	m.agentSnapshot = m.coordinator.List()
}
