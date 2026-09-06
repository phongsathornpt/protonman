package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	defaultMaxConcurrency = 4
	defaultMaxDepth       = 1
	defaultMaxRounds      = 10
	defaultMaxToolCalls   = turn.DefaultMaxToolCalls
	defaultMaxRuntime     = 30 * time.Minute
	defaultWaitTimeout    = 30 * time.Second
	defaultQueueTimeout   = 30 * time.Second
	defaultMaxLiveAgents  = 16
	defaultResultTTL      = 10 * time.Minute
	defaultCloseTimeout   = 5 * time.Second
	eventEmitTimeout      = 100 * time.Millisecond
	maxSummaryBytes       = 32 * 1024 // 32KB bound for child summaries returned to parent
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

type activeEntry struct {
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
	activeMu sync.RWMutex
	active   map[string]*activeEntry
	wg       sync.WaitGroup
	rootCtx  context.Context
	rootStop context.CancelFunc

	maxDepth            int
	maxRounds           int
	maxToolCalls        int
	maxLiveAgents       int
	maxRuntime          time.Duration
	waitTimeout         time.Duration
	defaultQueueTimeout time.Duration
	resultTTL           time.Duration
	closeTimeout        time.Duration
	eventSink           EventSink
	runnerFactory       RunnerFactory

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
		active:              make(map[string]*activeEntry),
		rootCtx:             rootCtx,
		rootStop:            rootStop,
		maxDepth:            defaultMaxDepth,
		maxRounds:           defaultMaxRounds,
		maxToolCalls:        defaultMaxToolCalls,
		maxLiveAgents:       defaultMaxLiveAgents,
		maxRuntime:          defaultMaxRuntime,
		waitTimeout:         defaultWaitTimeout,
		defaultQueueTimeout: defaultQueueTimeout,
		resultTTL:           defaultResultTTL,
		closeTimeout:        defaultCloseTimeout,
	}
	for _, opt := range options {
		if opt != nil {
			opt(c)
		}
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

	c.activeMu.Lock()
	if c.closed.Load() {
		c.activeMu.Unlock()
		return Handle{}, errors.New("coordinator is closed")
	}
	c.pruneExpiredLocked(time.Now())
	live := 0
	for _, existing := range c.active {
		if !existing.status.State.Terminal() {
			live++
		}
	}
	if c.maxLiveAgents > 0 && live >= c.maxLiveAgents {
		c.activeMu.Unlock()
		return Handle{}, fmt.Errorf("maximum live subagents reached (%d)", c.maxLiveAgents)
	}
	id := strings.TrimSpace(req.ID)
	if id == "" {
		id = fmt.Sprintf("%s-%d", req.Profile, atomic.AddUint64(&c.seq, 1))
		req.ID = id
	} else if _, exists := c.active[id]; exists {
		c.activeMu.Unlock()
		return Handle{}, fmt.Errorf("subagent %q already exists", id)
	}

	queuedAt := time.Now()
	runCtx, runCancel := context.WithCancel(c.rootCtx)
	entry := &activeEntry{
		status: AgentStatus{
			ID: id, ParentID: req.ParentID, Profile: req.Profile, Task: req.Task,
			State: StateQueued, StartTime: queuedAt, Depth: req.Depth,
		},
		cancel:  runCancel,
		done:    make(chan struct{}),
		started: make(chan struct{}),
	}
	c.active[id] = entry
	c.activeMu.Unlock()

	c.emit(runCtx, Event{Kind: EventAgentQueued, AgentID: id, ParentID: req.ParentID, Profile: req.Profile, Message: req.Task})
	c.wg.Add(1)
	go c.runEntry(runCtx, entry, req, queuedAt)
	return Handle{ID: id, Profile: req.Profile}, nil
}

func (c *Coordinator) runEntry(runCtx context.Context, entry *activeEntry, req Request, queuedAt time.Time) {
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
	c.activeMu.Lock()
	entry.status.State = StateRunning
	entry.status.StartedAt = startedAt
	close(entry.started)
	c.activeMu.Unlock()
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

func (c *Coordinator) finishEntry(entry *activeEntry, req Request, queuedAt, startedAt time.Time, err error) {
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

func (c *Coordinator) storeTerminal(entry *activeEntry, res Result, err error) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
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

// Wait waits for a subagent for at most timeout. A wait timeout never cancels
// the child; it returns the current state so callers can wait again later.
func (c *Coordinator) Wait(ctx context.Context, id string, timeout time.Duration) (WaitResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.activeMu.RLock()
	entry := c.active[strings.TrimSpace(id)]
	if entry == nil {
		c.activeMu.RUnlock()
		return WaitResult{}, fmt.Errorf("subagent %q not found", id)
	}
	done := entry.done
	state := entry.status.State
	c.activeMu.RUnlock()
	if state.Terminal() {
		return c.waitSnapshot(id)
	}

	if timeout <= 0 {
		timeout = c.waitTimeout
	}
	waitCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	select {
	case <-done:
		return c.waitSnapshot(id)
	case <-waitCtx.Done():
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			return c.waitSnapshot(id)
		}
		return WaitResult{}, waitCtx.Err()
	}
}

func (c *Coordinator) waitSnapshot(id string) (WaitResult, error) {
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()
	entry := c.active[id]
	if entry == nil {
		return WaitResult{}, fmt.Errorf("subagent %q not found", id)
	}
	wr := WaitResult{State: entry.status.State}
	if entry.status.State.Terminal() {
		res := entry.result
		wr.Result = &res
	}
	return wr, nil
}

func (c *Coordinator) pruneExpired() {
	if c.resultTTL <= 0 {
		return
	}
	c.activeMu.Lock()
	c.pruneExpiredLocked(time.Now())
	c.activeMu.Unlock()
}

func (c *Coordinator) pruneExpiredLocked(now time.Time) {
	if c.resultTTL <= 0 {
		return
	}
	for id, entry := range c.active {
		if entry.status.State.Terminal() && !entry.status.FinishedAt.IsZero() && now.Sub(entry.status.FinishedAt) >= c.resultTTL {
			delete(c.active, id)
		}
	}
}

// Get returns one retained subagent status.
func (c *Coordinator) Get(id string) (AgentStatus, bool) {
	c.pruneExpired()
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()
	entry, ok := c.active[strings.TrimSpace(id)]
	if !ok {
		return AgentStatus{}, false
	}
	return entry.status, true
}

// Lookup returns one retained status and, when terminal, its result.
func (c *Coordinator) Lookup(id string) (AgentStatus, *Result, bool) {
	c.pruneExpired()
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()
	entry, ok := c.active[strings.TrimSpace(id)]
	if !ok {
		return AgentStatus{}, nil, false
	}
	status := entry.status
	if !status.State.Terminal() {
		return status, nil, true
	}
	res := entry.result
	return status, &res, true
}

// Active returns queued or running subagents only.
func (c *Coordinator) Active() []AgentStatus {
	c.pruneExpired()
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()
	out := make([]AgentStatus, 0, len(c.active))
	for _, entry := range c.active {
		if !entry.status.State.Terminal() {
			out = append(out, entry.status)
		}
	}
	return out
}

// List returns all retained subagents, including terminal results.
func (c *Coordinator) List() []AgentStatus {
	c.pruneExpired()
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()
	out := make([]AgentStatus, 0, len(c.active))
	for _, entry := range c.active {
		out = append(out, entry.status)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartTime.Equal(out[j].StartTime) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartTime.Before(out[j].StartTime)
	})
	return out
}

// Cancel explicitly requests cancellation of one subagent.
func (c *Coordinator) Cancel(id string) error {
	c.activeMu.RLock()
	entry := c.active[strings.TrimSpace(id)]
	if entry == nil {
		c.activeMu.RUnlock()
		return fmt.Errorf("subagent %q not found", id)
	}
	terminal := entry.status.State.Terminal()
	cancel := entry.cancel
	c.activeMu.RUnlock()
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
	c.activeMu.RLock()
	e := c.active[h.ID]
	started, done := e.started, e.done
	c.activeMu.RUnlock()
	select {
	case <-done:
		return c.blockingResult(h.ID, h.Profile)
	case <-started:
	case <-ctx.Done():
		_ = c.Cancel(h.ID)
		return Result{AgentID: h.ID, Profile: h.Profile, Err: ctx.Err()}, ctx.Err()
	}
	c.activeMu.RLock()
	terminal := e.status.State.Terminal()
	c.activeMu.RUnlock()
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
	c.activeMu.Lock()
	c.closed.Store(true)
	c.rootStop()
	for _, entry := range c.active {
		if !entry.status.State.Terminal() {
			entry.cancel()
		}
	}
	c.activeMu.Unlock()

	deadline := time.NewTimer(c.closeTimeout)
	defer deadline.Stop()
	done := make(chan struct{})
	go func() { c.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-deadline.C:
		c.activeMu.RLock()
		remaining := 0
		for _, entry := range c.active {
			if !entry.status.State.Terminal() {
				remaining++
			}
		}
		c.activeMu.RUnlock()
		return fmt.Errorf("coordinator close timed out with %d active subagent(s)", remaining)
	}
}

// SetParentRegistry sets or updates the parent tool registry for scoping subagent tools.
func (c *Coordinator) SetParentRegistry(registry tool.Registry) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	c.parentRegistry = registry
}

// SetClient dynamically updates the model client used by child subagents.
func (c *Coordinator) SetClient(client model.Client) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	c.client = client
}

// Client returns the current model client used by child subagents.
func (c *Coordinator) Client() model.Client {
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()
	return c.client
}

// SetPermissionMode dynamically updates the permission mode for subagents.
func (c *Coordinator) SetPermissionMode(mode permission.Mode) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	if mode.Valid() {
		c.permissionMode = mode
	}
}

// PermissionMode returns the active permission mode configured for subagents.
func (c *Coordinator) PermissionMode() permission.Mode {
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()
	return c.permissionMode
}

// SetPrompt dynamically updates the interactive permission prompt resolver for subagents.
func (c *Coordinator) SetPrompt(prompt toolcall.PermissionPrompt) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	c.prompt = prompt
}

// SetCallGuard dynamically updates the execution guard (e.g. plan mode) for subagents.
func (c *Coordinator) SetCallGuard(guard toolcall.CallGuard) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	c.guard = guard
}

// CallGuard returns the active call guard for subagents.
func (c *Coordinator) CallGuard() toolcall.CallGuard {
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()
	return c.guard
}

func (c *Coordinator) emit(ctx context.Context, ev Event) {
	if c.eventSink == nil {
		return
	}
	emitCtx, cancel := context.WithTimeout(ctx, eventEmitTimeout)
	defer cancel()
	done := make(chan struct{}, 1)
	go func() {
		_ = c.eventSink(emitCtx, ev)
		done <- struct{}{}
	}()
	select {
	case <-done:
	case <-emitCtx.Done():
	}
}

func (c *Coordinator) execute(ctx context.Context, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{AgentID: req.ID, Profile: req.Profile}, err
	}

	c.activeMu.RLock()
	parentRegistry := c.parentRegistry
	client := c.client
	permMode := c.permissionMode
	prompt := c.prompt
	guard := c.guard
	c.activeMu.RUnlock()

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
