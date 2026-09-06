package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/projectTHORN/proton/internal/contextutil"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
	"github.com/projectTHORN/proton/internal/workspace"
)

const (
	defaultMaxConcurrency    = 4
	defaultMaxDepth          = 1
	defaultMaxRounds         = 10
	defaultMaxToolCalls      = turn.DefaultMaxToolCalls
	defaultMaxRuntime        = 30 * time.Minute
	defaultWaitTimeout       = 30 * time.Second
	defaultQueueTimeout      = 30 * time.Second
	defaultMaxLiveAgents     = 16
	defaultMaxRetainedAgents = 64
	defaultResultTTL         = 10 * time.Minute
	defaultEventQueueSize    = 64
	defaultCloseTimeout      = 5 * time.Second
	eventEmitTimeout         = 100 * time.Millisecond
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
	Depth      int       `json:"depth"`
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
	client         model.Client
	parentRegistry tool.Registry
	workspace      *workspace.Workspace
	policy         *permission.Policy

	permissionMode permission.Mode
	prompt         toolcall.PermissionPrompt
	guard          toolcall.CallGuard

	sem      chan struct{}
	wsGate   chan struct{}
	wsWriter chan struct{}
	agentsMu sync.RWMutex
	agents   map[string]*agentEntry
	wg       sync.WaitGroup
	rootCtx  context.Context
	rootStop context.CancelFunc

	maxDepth            int
	maxRounds           int
	maxToolCalls        int
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
		}
	}
}

// WithMaxDepth sets the maximum delegation nesting depth.
func WithMaxDepth(depth int) Option {
	return func(c *Coordinator) {
		if depth >= 0 {
			c.maxDepth = depth
		}
	}
}

// WithMaxRounds sets the maximum model/tool rounds per subagent.
func WithMaxRounds(rounds int) Option {
	return func(c *Coordinator) {
		if rounds >= 0 {
			c.maxRounds = rounds
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

// WithDefaultTimeout is a compatibility alias for WithMaxRuntime.
func WithDefaultTimeout(d time.Duration) Option { return WithMaxRuntime(d) }

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

// WithCallGuard attaches an execution guard (e.g. read-only plan mode) to subagents.
func WithCallGuard(guard toolcall.CallGuard) Option {
	return func(c *Coordinator) {
		c.guard = guard
	}
}

// NewCoordinator creates an agent coordinator for managing subagent goroutines.
func NewCoordinator(
	client model.Client,
	parentRegistry tool.Registry,
	ws *workspace.Workspace,
	policy *permission.Policy,
	options ...Option,
) *Coordinator {
	rootCtx, rootStop := context.WithCancel(context.Background())
	c := &Coordinator{
		client:              client,
		parentRegistry:      parentRegistry,
		workspace:           ws,
		policy:              policy,
		sem:                 make(chan struct{}, defaultMaxConcurrency),
		wsGate:              make(chan struct{}, defaultMaxConcurrency),
		wsWriter:            make(chan struct{}, 1),
		agents:              make(map[string]*agentEntry),
		rootCtx:             rootCtx,
		rootStop:            rootStop,
		maxDepth:            defaultMaxDepth,
		maxRounds:           defaultMaxRounds,
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
func (c *Coordinator) Spawn(ctx context.Context, req Request) (Handle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Handle{}, err
	}
	if err := req.Validate(); err != nil {
		return Handle{}, fmt.Errorf("invalid subagent request: %w", err)
	}
	if req.Depth > c.maxDepth {
		return Handle{}, fmt.Errorf("delegation depth %d exceeds maximum depth %d", req.Depth, c.maxDepth)
	}

	c.agentsMu.Lock()
	if c.closed.Load() {
		c.agentsMu.Unlock()
		return Handle{}, errors.New("coordinator is closed")
	}
	c.pruneExpiredLocked(time.Now())
	live := 0
	for _, existing := range c.agents {
		if !existing.status.State.Terminal() {
			live++
		}
	}
	if c.maxLiveAgents > 0 && live >= c.maxLiveAgents {
		c.agentsMu.Unlock()
		return Handle{}, fmt.Errorf("maximum live subagents reached (%d)", c.maxLiveAgents)
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = fmt.Sprintf("%s-%d", req.Profile, atomic.AddUint64(&c.seq, 1))
		req.ID = id
	} else if _, exists := c.agents[id]; exists {
		c.agentsMu.Unlock()
		return Handle{}, fmt.Errorf("subagent %q already exists", id)
	}

	queuedAt := time.Now()
	runCtx, runCancel := context.WithCancel(c.rootCtx)
	entry := &agentEntry{
		status: AgentStatus{
			ID: id, ParentID: req.ParentID, Profile: req.Profile, Task: req.Task,
			State: StateQueued, StartTime: queuedAt, Depth: req.Depth,
		},
		cancel:  runCancel,
		done:    make(chan struct{}),
		started: make(chan struct{}),
	}
	c.agents[id] = entry
	c.agentsMu.Unlock()

	c.emit(runCtx, Event{Kind: EventAgentQueued, AgentID: id, ParentID: req.ParentID, Profile: req.Profile, Message: req.Task})
	c.wg.Add(1)
	go c.runEntry(runCtx, entry, req, queuedAt)
	return Handle{ID: id, Profile: req.Profile}, nil
}

func (c *Coordinator) runEntry(runCtx context.Context, entry *agentEntry, req Request, queuedAt time.Time) {
	defer c.wg.Done()
	defer entry.cancel()
	defer close(entry.done)

	executionTimeout := req.Timeout
	if executionTimeout == 0 || (c.maxRuntime > 0 && executionTimeout > c.maxRuntime) {
		executionTimeout = c.maxRuntime
	}
	queueTimeout := req.QueueTimeout
	if queueTimeout == 0 {
		queueTimeout = c.defaultQueueTimeout
	}
	queueCtx := runCtx
	queueCancel := func() {}
	if queueTimeout > 0 {
		queueCtx, queueCancel = context.WithTimeout(runCtx, queueTimeout)
	}
	defer queueCancel()

	select {
	case c.sem <- struct{}{}:
	case <-queueCtx.Done():
		c.finishEntry(entry, req, queuedAt, time.Time{}, queueCtx.Err())
		return
	}
	defer func() { <-c.sem }()

	releaseWorkspace, err := c.acquireWorkspace(queueCtx, req.Profile.IsMutating())
	if err != nil {
		c.finishEntry(entry, req, queuedAt, time.Time{}, err)
		return
	}
	defer releaseWorkspace()
	queueCancel()

	startedAt := time.Now()
	c.agentsMu.Lock()
	entry.status.State = StateRunning
	entry.status.StartedAt = startedAt
	close(entry.started)
	c.agentsMu.Unlock()
	queueDuration := startedAt.Sub(queuedAt)

	execCtx := runCtx
	execCancel := func() {}
	if executionTimeout > 0 {
		execCtx, execCancel = context.WithTimeout(runCtx, executionTimeout)
	}
	defer execCancel()

	c.emit(execCtx, Event{Kind: EventAgentStarted, AgentID: req.ID, ParentID: req.ParentID, Profile: req.Profile, Message: req.Task, QueueDuration: queueDuration})
	res, runErr := c.execute(execCtx, req)
	res.QueueDuration = queueDuration
	res.Duration = time.Since(startedAt)
	res.TotalDuration = time.Since(queuedAt)
	if runErr != nil {
		res.Err = runErr
	}
	c.storeTerminal(entry, res, runErr)

	eventKind := EventAgentCompleted
	if runErr != nil {
		eventKind = EventAgentFailed
	}
	emitCtx, emitDone := contextutil.DetachedTimeout(execCtx, 5*time.Second)
	c.emit(emitCtx, Event{Kind: eventKind, AgentID: req.ID, ParentID: req.ParentID, Profile: req.Profile, Message: res.Summary, QueueDuration: res.QueueDuration, Duration: res.Duration, TotalDuration: res.TotalDuration, Err: runErr})
	emitDone()
}

func (c *Coordinator) finishEntry(entry *agentEntry, req Request, queuedAt, startedAt time.Time, err error) {
	now := time.Now()
	res := Result{AgentID: req.ID, Profile: req.Profile, QueueDuration: now.Sub(queuedAt), TotalDuration: now.Sub(queuedAt), Err: err}
	if !startedAt.IsZero() {
		res.Duration = now.Sub(startedAt)
	}
	c.storeTerminal(entry, res, err)
	c.emit(c.rootCtx, Event{Kind: EventAgentFailed, AgentID: req.ID, ParentID: req.ParentID, Profile: req.Profile, QueueDuration: res.QueueDuration, Duration: res.Duration, TotalDuration: res.TotalDuration, Err: err})
	if startedAt.IsZero() {
		close(entry.started)
	}
}

func (c *Coordinator) storeTerminal(entry *agentEntry, res Result, err error) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	entry.result = res
	entry.err = err
	entry.status.FinishedAt = time.Now()
	switch {
	case err == nil:
		entry.status.State = StateCompleted
	case errors.Is(err, context.Canceled):
		entry.status.State = StateCanceled
	default:
		entry.status.State = StateFailed
	}
}

// Cancel explicitly requests cancellation of one subagent.
func (c *Coordinator) Cancel(id string) error {
	c.agentsMu.RLock()
	entry := c.agents[strings.TrimSpace(id)]
	if entry == nil {
		c.agentsMu.RUnlock()
		return fmt.Errorf("subagent %q not found", id)
	}
	terminal := entry.status.State.Terminal()
	cancel := entry.cancel
	c.agentsMu.RUnlock()
	if terminal {
		return nil
	}
	cancel()
	return nil
}

// Run preserves blocking compatibility while keeping queue and execution budgets separate.
func (c *Coordinator) Run(ctx context.Context, req Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	h, err := c.Spawn(ctx, req)
	if err != nil {
		return Result{}, err
	}
	c.agentsMu.RLock()
	e := c.agents[h.ID]
	started, done := e.started, e.done
	c.agentsMu.RUnlock()
	select {
	case <-done:
		return c.blockingResult(h.ID, h.Profile)
	case <-started:
	case <-ctx.Done():
		_ = c.Cancel(h.ID)
		return Result{AgentID: h.ID, Profile: h.Profile, Err: ctx.Err()}, ctx.Err()
	}
	c.agentsMu.RLock()
	terminal := e.status.State.Terminal()
	c.agentsMu.RUnlock()
	if terminal {
		return c.blockingResult(h.ID, h.Profile)
	}
	t := req.Timeout
	if t == 0 || (c.maxRuntime > 0 && t > c.maxRuntime) {
		t = c.maxRuntime
	}
	wctx, cancel := boundedWaitContext(ctx, t)
	defer cancel()
	select {
	case <-done:
		return c.blockingResult(h.ID, h.Profile)
	case <-wctx.Done():
		_ = c.Cancel(h.ID)
		err := wctx.Err()
		return Result{AgentID: h.ID, Profile: h.Profile, Err: err}, err
	}
}

func boundedWaitContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (c *Coordinator) blockingResult(id string, profile Profile) (Result, error) {
	wr, err := c.waitSnapshot(id)
	if err != nil || wr.Result == nil {
		return Result{AgentID: id, Profile: profile}, err
	}
	return *wr.Result, wr.Result.Err
}

func (c *Coordinator) acquireWorkspace(ctx context.Context, exclusive bool) (func(), error) {
	count := 1
	writerHeld := false
	if exclusive {
		select {
		case c.wsWriter <- struct{}{}:
			writerHeld = true
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		count = cap(c.wsGate)
	}
	acquired := 0
	for acquired < count {
		select {
		case c.wsGate <- struct{}{}:
			acquired++
		case <-ctx.Done():
			for acquired > 0 {
				<-c.wsGate
				acquired--
			}
			if writerHeld {
				<-c.wsWriter
			}
			return nil, ctx.Err()
		}
	}
	return func() {
		for released := 0; released < count; released++ {
			<-c.wsGate
		}
		if writerHeld {
			<-c.wsWriter
		}
	}, nil
}

// Close cancels all non-terminal subagents and waits for workers to exit.
func (c *Coordinator) Close() error {
	c.agentsMu.Lock()
	c.closed.Store(true)
	c.rootStop()
	for _, entry := range c.agents {
		if !entry.status.State.Terminal() {
			entry.cancel()
		}
	}
	c.agentsMu.Unlock()

	c.closeOnce.Do(func() {
		go func() { c.wg.Wait(); close(c.closeDone) }()
	})
	deadline := time.NewTimer(c.closeTimeout)
	defer deadline.Stop()
	select {
	case <-c.closeDone:
		return nil
	case <-deadline.C:
		c.agentsMu.RLock()
		remaining := 0
		for _, entry := range c.agents {
			if !entry.status.State.Terminal() {
				remaining++
			}
		}
		c.agentsMu.RUnlock()
		return fmt.Errorf("coordinator close timed out with %d active subagent(s)", remaining)
	}
}

// SetParentRegistry sets or updates the parent tool registry for scoping subagent tools.
func (c *Coordinator) SetParentRegistry(registry tool.Registry) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.parentRegistry = registry
}

// SetClient dynamically updates the model client used by child subagents.
func (c *Coordinator) SetClient(client model.Client) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.client = client
}

// Client returns the current model client used by child subagents.
func (c *Coordinator) Client() model.Client {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	return c.client
}

// SetPermissionMode dynamically updates the permission mode for subagents.
func (c *Coordinator) SetPermissionMode(mode permission.Mode) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	if mode.Valid() {
		c.permissionMode = mode
	}
}

// PermissionMode returns the active permission mode configured for subagents.
func (c *Coordinator) PermissionMode() permission.Mode {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	return c.permissionMode
}

// SetPrompt dynamically updates the interactive permission prompt resolver for subagents.
func (c *Coordinator) SetPrompt(prompt toolcall.PermissionPrompt) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.prompt = prompt
}

// SetCallGuard dynamically updates the execution guard (e.g. plan mode) for subagents.
func (c *Coordinator) SetCallGuard(guard toolcall.CallGuard) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.guard = guard
}

// CallGuard returns the active call guard for subagents.
func (c *Coordinator) CallGuard() toolcall.CallGuard {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	return c.guard
}

// Subscribe returns a bounded lifecycle stream. Slow subscribers drop events
// rather than blocking agent execution; callers resnapshot coordinator state on
// every delivered event, so events are wakeups rather than the source of truth.
func (c *Coordinator) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan Event, buffer)
	id := atomic.AddUint64(&c.subscriberSeq, 1)
	c.eventMu.Lock()
	c.subscribers[id] = ch
	c.eventMu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			c.eventMu.Lock()
			delete(c.subscribers, id)
			close(ch)
			c.eventMu.Unlock()
		})
	}
}

func (c *Coordinator) broadcast(ev Event) {
	c.eventMu.RLock()
	defer c.eventMu.RUnlock()
	for _, ch := range c.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}
func (c *Coordinator) emit(_ context.Context, ev Event) {
	c.broadcast(ev)
	if c.eventSink == nil {
		return
	}
	select {
	case c.eventQueue <- ev:
	default:
	}
}

func (c *Coordinator) runEventSink() {
	for {
		select {
		case ev := <-c.eventQueue:
			emitCtx, done := contextutil.DetachedTimeout(c.rootCtx, eventEmitTimeout)
			_ = c.eventSink(emitCtx, ev)
			done()
		case <-c.rootCtx.Done():
			return
		}
	}
}

func (c *Coordinator) execute(ctx context.Context, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{AgentID: req.ID, Profile: req.Profile}, err
	}

	c.agentsMu.RLock()
	parentRegistry := c.parentRegistry
	client := c.client
	permMode := c.permissionMode
	prompt := c.prompt
	guard := c.guard
	c.agentsMu.RUnlock()

	// 1. Build profile-scoped tool registry
	scopedRegistry := FilterRegistryForProfile(parentRegistry, req.Profile, req.Depth)

	// 2. Build scoped tool service
	// For workers: if parent mode is always-approve, inherit always-approve.
	// Otherwise, run in parent mode (or ModeAuto by default) and attach prompt.
	// For read-only: uses always-approve mode since tools are already restricted to safe reads.
	serviceMode := permission.ModeAlwaysApprove
	if req.Profile.IsMutating() {
		if permMode == permission.ModeAlwaysApprove {
			serviceMode = permission.ModeAlwaysApprove
		} else if permMode.Valid() {
			serviceMode = permMode
		} else {
			serviceMode = permission.ModeAuto
		}
	}

	policy := c.policy
	if policy == nil {
		p, err := permission.NewPolicy(permission.Config{})
		if err != nil {
			return Result{AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create default policy: %w", err)
		}
		policy = p
	}

	serviceOpts := []toolcall.Option{toolcall.WithMode(serviceMode)}
	if prompt != nil {
		serviceOpts = append(serviceOpts, toolcall.WithPrompt(prompt))
	}

	service, err := toolcall.NewService(
		scopedRegistry,
		policy,
		serviceOpts...,
	)
	if err != nil {
		return Result{AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create scoped tool service: %w", err)
	}
	if guard != nil {
		service.SetCallGuard(guard)
	}

	// 3. Resolve turn runner
	var runner turn.Runner
	if c.runnerFactory != nil {
		r, rerr := c.runnerFactory(req.Profile, service)
		if rerr != nil {
			return Result{AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create turn runner: %w", rerr)
		}
		runner = r
	} else {
		if client == nil {
			return Result{AgentID: req.ID, Profile: req.Profile}, errors.New("model client is required for subagent execution")
		}
		loop, lerr := turn.NewLoop(
			client,
			service,
			turn.WithMaxRounds(c.maxRounds),
			turn.WithMaxToolCalls(c.maxToolCalls),
		)
		if lerr != nil {
			return Result{AgentID: req.ID, Profile: req.Profile}, fmt.Errorf("create turn loop: %w", lerr)
		}
		runner = loop
	}

	// 4. Build messages
	systemContent := SystemPromptForProfile(req.Profile)
	messages := []model.Message{
		{Role: model.RoleSystem, Content: systemContent},
		{Role: model.RoleUser, Content: formatUserPrompt(req)},
	}

	// 5. Run turn
	turnResult, err := runner.Run(ctx, messages, func(_ context.Context, te turn.Event) error {
		if te.Kind == turn.EventToolCall {
			c.emit(ctx, Event{
				Kind:     EventAgentProgress,
				AgentID:  req.ID,
				ParentID: req.ParentID,
				Profile:  req.Profile,
				Message:  fmt.Sprintf("using %s", te.Call.Name),
			})
		}
		return nil
	})
	if err != nil {
		return Result{AgentID: req.ID, Profile: req.Profile, Rounds: turnResult.Rounds}, err
	}

	summary := strings.TrimSpace(turnResult.Message.Content)
	if summary == "" {
		summary = "Task completed with no final text response."
	}
	if len(summary) > maxSummaryBytes {
		summary = summary[:maxSummaryBytes] + "\n... [output truncated]"
	}

	return Result{
		AgentID: req.ID,
		Profile: req.Profile,
		Summary: summary,
		Rounds:  turnResult.Rounds,
	}, nil
}

func formatUserPrompt(req Request) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Task: %s\n", req.Task))
	if req.Context != "" {
		b.WriteString(fmt.Sprintf("\nContext:\n%s\n", req.Context))
	}
	return b.String()
}
