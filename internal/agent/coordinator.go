package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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
	defaultTimeout        = 5 * time.Minute
	maxSummaryBytes       = 32 * 1024 // 32KB bound for child summaries returned to parent
)

// RunnerFactory builds an injectable turn.Runner for a specific subagent run.
type RunnerFactory func(profile Profile, tools *toolcall.Service) (turn.Runner, error)

// AgentStatus describes the live state of an in-flight subagent.
type AgentStatus struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parent_id,omitempty"`
	Profile   Profile   `json:"profile"`
	Task      string    `json:"task"`
	StartTime time.Time `json:"start_time"`
	Depth     int       `json:"depth"`
}

type activeEntry struct {
	status AgentStatus
	cancel context.CancelFunc
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
	wsLock   sync.RWMutex
	activeMu sync.RWMutex
	active   map[string]*activeEntry
	wg       sync.WaitGroup

	maxDepth       int
	maxRounds      int
	defaultTimeout time.Duration
	eventSink      EventSink
	runnerFactory  RunnerFactory

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
		if rounds > 0 {
			c.maxRounds = rounds
		}
	}
}

// WithDefaultTimeout sets the default execution deadline for a subagent.
func WithDefaultTimeout(d time.Duration) Option {
	return func(c *Coordinator) {
		if d > 0 {
			c.defaultTimeout = d
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
	c := &Coordinator{
		client:         client,
		parentRegistry: parentRegistry,
		workspace:      ws,
		policy:         policy,
		sem:            make(chan struct{}, defaultMaxConcurrency),
		active:         make(map[string]*activeEntry),
		maxDepth:       defaultMaxDepth,
		maxRounds:      defaultMaxRounds,
		defaultTimeout: defaultTimeout,
	}
	for _, opt := range options {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Run dispatches a subagent execution into a goroutine and blocks until completion,
// timeout, or parent context cancellation.
func (c *Coordinator) Run(ctx context.Context, req Request) (Result, error) {
	c.activeMu.Lock()
	if c.closed.Load() {
		c.activeMu.Unlock()
		return Result{}, errors.New("coordinator is closed")
	}
	if err := req.Validate(); err != nil {
		c.activeMu.Unlock()
		return Result{}, fmt.Errorf("invalid subagent request: %w", err)
	}
	if req.Depth > c.maxDepth {
		c.activeMu.Unlock()
		return Result{}, fmt.Errorf("delegation depth %d exceeds maximum depth %d", req.Depth, c.maxDepth)
	}

	id := req.ID
	if strings.TrimSpace(id) == "" {
		id = fmt.Sprintf("%s-%d", req.Profile, atomic.AddUint64(&c.seq, 1))
		req.ID = id
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.defaultTimeout
	}

	childCtx, cancel := context.WithTimeout(ctx, timeout)

	status := AgentStatus{
		ID:        id,
		ParentID:  req.ParentID,
		Profile:   req.Profile,
		Task:      req.Task,
		StartTime: time.Now(),
		Depth:     req.Depth,
	}

	c.wg.Add(1)
	c.active[id] = &activeEntry{
		status: status,
		cancel: cancel,
	}
	c.activeMu.Unlock()

	defer func() {
		cancel()
		c.activeMu.Lock()
		delete(c.active, id)
		c.activeMu.Unlock()
	}()

	// Buffer size 1 guarantees the child goroutine never blocks on send even if
	// the parent context was canceled or timed out early.
	resultCh := make(chan Result, 1)

	go func() {
		defer c.wg.Done()

		// 1. Acquire concurrency semaphore
		select {
		case c.sem <- struct{}{}:
			defer func() { <-c.sem }()
		case <-childCtx.Done():
			resultCh <- Result{
				AgentID: id,
				Profile: req.Profile,
				Err:     childCtx.Err(),
			}
			return
		}

		// 2. Acquire workspace lock: read-only profiles take shared lock,
		// mutating worker takes exclusive lock to prevent file write collisions.
		if req.Profile.IsMutating() {
			c.wsLock.Lock()
			defer c.wsLock.Unlock()
		} else {
			c.wsLock.RLock()
			defer c.wsLock.RUnlock()
		}

		// 3. Emit started event
		start := time.Now()
		c.emit(childCtx, Event{
			Kind:     EventAgentStarted,
			AgentID:  id,
			ParentID: req.ParentID,
			Profile:  req.Profile,
			Message:  req.Task,
		})

		// 4. Run subagent
		res, err := c.execute(childCtx, req)
		res.Duration = time.Since(start)

		if err != nil {
			res.Err = err
			emitCtx, emitCancel := context.WithTimeout(context.WithoutCancel(childCtx), 5*time.Second)
			c.emit(emitCtx, Event{
				Kind:     EventAgentFailed,
				AgentID:  id,
				ParentID: req.ParentID,
				Profile:  req.Profile,
				Duration: res.Duration,
				Err:      err,
			})
			emitCancel()
		} else {
			c.emit(childCtx, Event{
				Kind:     EventAgentCompleted,
				AgentID:  id,
				ParentID: req.ParentID,
				Profile:  req.Profile,
				Duration: res.Duration,
				Message:  res.Summary,
			})
		}

		resultCh <- res
	}()

	select {
	case <-ctx.Done():
		// Parent canceled (e.g. user Ctrl+C or turn timeout).
		// Child goroutine will be cancelled via childCtx defer cancel().
		return Result{
			AgentID: id,
			Profile: req.Profile,
			Err:     ctx.Err(),
		}, ctx.Err()
	case res := <-resultCh:
		return res, res.Err
	}
}

// Active returns a snapshot of currently running subagents.
func (c *Coordinator) Active() []AgentStatus {
	c.activeMu.RLock()
	defer c.activeMu.RUnlock()

	out := make([]AgentStatus, 0, len(c.active))
	for _, entry := range c.active {
		out = append(out, entry.status)
	}
	return out
}

// Close cancels all active subagents and waits for all goroutines to exit.
func (c *Coordinator) Close() error {
	c.activeMu.Lock()
	if !c.closed.CompareAndSwap(false, true) {
		c.activeMu.Unlock()
		return nil
	}

	for _, entry := range c.active {
		entry.cancel()
	}
	c.activeMu.Unlock()

	c.wg.Wait()
	return nil
}

// SetParentRegistry sets or updates the parent tool registry for scoping subagent tools.
func (c *Coordinator) SetParentRegistry(registry tool.Registry) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	c.parentRegistry = registry
}

// SetClient sets or updates the model client for running subagents.
func (c *Coordinator) SetClient(client model.Client) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	c.client = client
}

// SetPermissionMode dynamically updates the permission mode for subagents.
func (c *Coordinator) SetPermissionMode(mode permission.Mode) {
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	if mode.Valid() {
		c.permissionMode = mode
	}
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

func (c *Coordinator) emit(ctx context.Context, ev Event) {
	if c.eventSink != nil {
		_ = c.eventSink(ctx, ev)
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

	// 1. Build profile-scoped tool registry with fine-grained workspace locking
	scopedRegistry := FilterRegistryForProfile(parentRegistry, req.Profile, req.Depth)
	lockedReg := newLockedRegistry(scopedRegistry, &c.wsLock)

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
		lockedReg,
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

type lockedRegistry struct {
	inner  tool.Registry
	wsLock *sync.RWMutex
}

var _ tool.Registry = (*lockedRegistry)(nil)

func newLockedRegistry(inner tool.Registry, wsLock *sync.RWMutex) tool.Registry {
	return &lockedRegistry{
		inner:  inner,
		wsLock: wsLock,
	}
}

func (r *lockedRegistry) Lookup(name string) (tool.Handler, bool) {
	h, ok := r.inner.Lookup(name)
	if !ok {
		return nil, false
	}
	return lockedHandler{
		inner:  h,
		wsLock: r.wsLock,
	}, true
}

func (r *lockedRegistry) Definitions() []tool.Definition {
	return r.inner.Definitions()
}

type lockedHandler struct {
	inner  tool.Handler
	wsLock *sync.RWMutex
}

var _ tool.Handler = lockedHandler{}
var _ tool.DetailProvider = lockedHandler{}

func (h lockedHandler) Definition() tool.Definition {
	return h.inner.Definition()
}

func (h lockedHandler) PermissionDetail(arguments json.RawMessage) string {
	if pd, ok := h.inner.(tool.DetailProvider); ok {
		return pd.PermissionDetail(arguments)
	}
	return ""
}

func (h lockedHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	def := h.inner.Definition()
	switch def.Kind {
	case tool.KindEdit, tool.KindBash:
		h.wsLock.Lock()
		defer h.wsLock.Unlock()
	case tool.KindRead, tool.KindGrep:
		h.wsLock.RLock()
		defer h.wsLock.RUnlock()
	}
	return h.inner.Execute(ctx, call)
}
