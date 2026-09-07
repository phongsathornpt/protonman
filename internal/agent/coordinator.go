package agent

import (
	"context"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"github.com/projectTHORN/proton/internal/skill"
	"sync"
	"sync/atomic"
	"time"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
	"github.com/projectTHORN/proton/internal/workspace"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

const (
	defaultMaxConcurrency    = 4
	defaultMaxToolCalls      = runtimepolicy.TurnMaxToolCalls
	defaultMaxRuntime        = runtimepolicy.AgentMaxRuntime
	defaultWaitTimeout       = runtimepolicy.AgentWaitTimeout
	defaultQueueTimeout      = runtimepolicy.AgentQueueTimeout
	defaultMaxLiveAgents     = runtimepolicy.AgentMaxLive
	defaultMaxRetainedAgents = runtimepolicy.AgentMaxRetained
	defaultResultTTL         = runtimepolicy.AgentResultTTL
	defaultEventQueueSize    = 64
	defaultCloseTimeout      = runtimepolicy.AgentCloseTimeout
	eventEmitTimeout         = runtimepolicy.AgentEventEmitTimeout
	maxSummaryBytes          = 32 * 1024 // 32KB bound for child summaries returned to parent
)

// RunnerFactory builds an injectable turn.Runner for a specific subagent run.
type RunnerFactory func(profile Profile, tools *toolcall.Service) (turn.Runner, error)

// AgentStatus describes the live state of an in-flight subagent.
type AgentStatus struct {
	ID         string    `json:"id"`
	ParentID   string    `json:"parent_id,omitempty"`
	Profile    Profile   `json:"profile"`
	Task       string    `json:"task"`
	State      State     `json:"state"`
	StartTime  time.Time `json:"start_time"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Reason     string    `json:"reason,omitempty"`
}

type agentEntry struct {
	status  AgentStatus
	cancel  context.CancelFunc
	done    chan struct{}
	started chan struct{}
	result  Result
	err     error
}

// Coordinator manages subagent execution in bounded, cancellable goroutines.
type Coordinator struct {
	languageModel  sdk.LanguageModel
	parentRegistry tool.Registry
	skillRegistry  *skill.Registry
	workspace      *workspace.Workspace
	policy         *permission.Policy

	permissionMode permission.Mode
	prompt         toolcall.PermissionPrompt
	guard          toolcall.CallGuard

	sem         chan struct{}
	wsGate      chan struct{}
	wsWriter    chan struct{}
	wsAdmission chan struct{}
	agentsMu    sync.RWMutex
	agents      map[string]*agentEntry
	wg          sync.WaitGroup
	rootCtx     context.Context
	rootStop    context.CancelFunc

	maxToolCalls        int
	reasoningEffort     sdk.ReasoningEffort
	maxLiveAgents       int
	maxRetainedAgents   int
	maxRuntime          time.Duration
	waitTimeout         time.Duration
	defaultQueueTimeout time.Duration
	resultTTL           time.Duration
	closeTimeout        time.Duration
	eventSink           EventSink
	runnerFactory       RunnerFactory
	eventQueue          chan Event
	closeOnce           sync.Once
	closeDone           chan struct{}
	eventMu             sync.RWMutex
	subscribers         map[uint64]chan Event
	subscriberSeq       uint64

	seq    uint64
	closed atomic.Bool
}

// Option configures a Coordinator.
type Option func(*Coordinator)

// WithMaxConcurrency sets the maximum number of concurrent subagent goroutines.
func WithMaxConcurrency(n int) Option {
	return func(c *Coordinator) {
		if n > 0 {
			c.sem = make(chan struct{}, n)
			c.wsGate = make(chan struct{}, n)
			c.wsWriter = make(chan struct{}, 1)
			c.wsAdmission = make(chan struct{}, 1)
		}
	}
}

// WithReasoningEffort overrides portable profile reasoning for subagents.
func WithReasoningEffort(effort sdk.ReasoningEffort) Option {
	return func(c *Coordinator) {
		if effort.Valid() {
			c.reasoningEffort = effort
		}
	}
}

// WithMaxToolCalls sets the cumulative tool-call limit per subagent.
func WithMaxToolCalls(calls int) Option {
	return func(c *Coordinator) {
		if calls >= 0 {
			c.maxToolCalls = calls
		}
	}
}

// WithMaxRuntime sets the hard safety ceiling for one spawned subagent.
func WithMaxRuntime(d time.Duration) Option {
	return func(c *Coordinator) {
		if d > 0 {
			c.maxRuntime = d
		}
	}
}

// WithDefaultWaitTimeout sets the default non-destructive wait duration.
func WithDefaultWaitTimeout(d time.Duration) Option {
	return func(c *Coordinator) {
		if d > 0 {
			c.waitTimeout = d
		}
	}
}

// WithMaxLiveAgents bounds queued and running subagents.
func WithMaxLiveAgents(n int) Option {
	return func(c *Coordinator) {
		if n > 0 {
			c.maxLiveAgents = n
		}
	}
}

// WithMaxRetainedAgents bounds terminal lifecycle records retained for lookup.
func WithMaxRetainedAgents(n int) Option {
	return func(c *Coordinator) {
		if n > 0 {
			c.maxRetainedAgents = n
		}
	}
}

// WithResultTTL controls how long terminal results remain queryable.
func WithResultTTL(d time.Duration) Option {
	return func(c *Coordinator) {
		if d > 0 {
			c.resultTTL = d
		}
	}
}

// WithDefaultQueueTimeout sets the maximum time a subagent may wait for
// concurrency and workspace capacity before execution starts.
func WithDefaultQueueTimeout(d time.Duration) Option {
	return func(c *Coordinator) {
		if d >= 0 {
			c.defaultQueueTimeout = d
		}
	}
}

// WithCloseTimeout bounds graceful coordinator shutdown after cancellation.
func WithCloseTimeout(d time.Duration) Option {
	return func(c *Coordinator) {
		if d > 0 {
			c.closeTimeout = d
		}
	}
}

// WithEventSink attaches an observer for subagent lifecycle events.
func WithEventSink(sink EventSink) Option {
	return func(c *Coordinator) {
		c.eventSink = sink
	}
}

// WithRunnerFactory configures a custom factory for creating turn runners (useful for testing).
func WithRunnerFactory(factory RunnerFactory) Option {
	return func(c *Coordinator) {
		c.runnerFactory = factory
	}
}

// WithPermissionMode sets the baseline permission mode for subagents.
func WithPermissionMode(mode permission.Mode) Option {
	return func(c *Coordinator) {
		if mode.Valid() {
			c.permissionMode = mode
		}
	}
}

// WithPermissionPrompt attaches an interactive prompt resolver for subagent tool calls.
func WithPermissionPrompt(prompt toolcall.PermissionPrompt) Option {
	return func(c *Coordinator) {
		c.prompt = prompt
	}
}

// WithSkillRegistry supplies the immutable skill catalog used to create isolated child skill sessions.
func WithSkillRegistry(registry *skill.Registry) Option {
	return func(c *Coordinator) {
		c.skillRegistry = registry
	}
}

// WithCallGuard attaches an execution guard (e.g. read-only plan mode) to subagents.
func WithCallGuard(guard toolcall.CallGuard) Option {
	return func(c *Coordinator) {
		c.guard = guard
	}
}

// NewCoordinator creates an agent coordinator for managing subagent goroutines.
func NewCoordinator(
	languageModel sdk.LanguageModel,
	parentRegistry tool.Registry,
	ws *workspace.Workspace,
	policy *permission.Policy,
	options ...Option,
) *Coordinator {
	rootCtx, rootStop := context.WithCancel(context.Background())
	c := &Coordinator{
		languageModel:       languageModel,
		parentRegistry:      parentRegistry,
		workspace:           ws,
		policy:              policy,
		sem:                 make(chan struct{}, defaultMaxConcurrency),
		wsGate:              make(chan struct{}, defaultMaxConcurrency),
		wsWriter:            make(chan struct{}, 1),
		wsAdmission:         make(chan struct{}, 1),
		agents:              make(map[string]*agentEntry),
		rootCtx:             rootCtx,
		rootStop:            rootStop,
		maxToolCalls:        defaultMaxToolCalls,
		maxLiveAgents:       defaultMaxLiveAgents,
		maxRetainedAgents:   defaultMaxRetainedAgents,
		maxRuntime:          defaultMaxRuntime,
		waitTimeout:         defaultWaitTimeout,
		defaultQueueTimeout: defaultQueueTimeout,
		resultTTL:           defaultResultTTL,
		closeTimeout:        defaultCloseTimeout,
		subscribers:         make(map[uint64]chan Event),
		eventQueue:          make(chan Event, defaultEventQueueSize),
		closeDone:           make(chan struct{}),
	}
	for _, opt := range options {
		if opt != nil {
			opt(c)
		}
	}
	if c.eventSink != nil {
		go c.runEventSink()
	}
	return c
}

// Spawn validates and schedules a subagent whose lifetime is owned by the coordinator.
// The caller context only bounds submission; canceling it after Spawn returns does not
// cancel the child. Use Cancel for explicit cancellation.
