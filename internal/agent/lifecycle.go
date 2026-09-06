package agent

import (
	"context"
	"errors"
	"fmt"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/projectTHORN/proton/internal/contextutil"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

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
	c.agentsMu.Lock()
	if c.closed.Load() {
		c.agentsMu.Unlock()
		return Handle{}, ErrCoordinatorClosed
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
		return Handle{}, fmt.Errorf("%w (%d)", ErrLiveLimit, c.maxLiveAgents)
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
			State: StateQueued, StartTime: queuedAt,
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
	if err := runCtx.Err(); err != nil {
		c.finishEntry(entry, req, queuedAt, time.Time{}, err)
		return
	}

	startedAt := time.Now()
	c.agentsMu.Lock()
	if entry.status.State == StateCanceling || runCtx.Err() != nil {
		c.agentsMu.Unlock()
		c.finishEntry(entry, req, queuedAt, time.Time{}, context.Canceled)
		return
	}
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
	emitCtx, emitDone := contextutil.DetachedTimeout(execCtx, runtimepolicy.AgentLifecycleEmitTimeout)
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
	entry.status.Reason = terminalReason(err)
	switch {
	case err == nil:
		entry.status.State = StateCompleted
	case errors.Is(err, context.Canceled):
		entry.status.State = StateCanceled
	default:
		entry.status.State = StateFailed
	}
}

func terminalReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, turn.ErrMaxRounds):
		return "max rounds reached"
	case errors.Is(err, ErrUnverifiedChanges):
		return "unverified changes"
	case errors.Is(err, toolcall.ErrPermissionDenied):
		return "permission denied"
	}
	var providerErr *sdk.ProviderError
	if errors.As(err, &providerErr) {
		switch providerErr.Kind {
		case sdk.ErrorAuthentication:
			return "authentication failed"
		case sdk.ErrorPermission:
			return "permission denied"
		case sdk.ErrorRateLimit:
			return "rate limited"
		case sdk.ErrorModelNotFound:
			return "model unavailable"
		case sdk.ErrorContextLength:
			return "context limit exceeded"
		case sdk.ErrorOverloaded:
			return "provider overloaded"
		case sdk.ErrorTransport:
			return "network error"
		case sdk.ErrorProtocol:
			return "provider protocol error"
		case sdk.ErrorInvalidRequest:
			return "invalid model request"
		default:
			return "model error"
		}
	}
	msg := strings.ToValidUTF8(strings.TrimSpace(err.Error()), "�")
	if len(msg) <= 80 {
		return msg
	}
	cut := 77
	for cut > 0 && !utf8.ValidString(msg[:cut]) {
		cut--
	}
	return msg[:cut] + "..."
}

// Cancel explicitly requests cancellation of one subagent.
// CancelByParent requests cancellation for all non-terminal subagents owned by parentID.
// It returns the number of cancellation requests issued.
func (c *Coordinator) CancelByParent(parentID string) int {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return 0
	}
	c.agentsMu.Lock()
	cancels := make([]context.CancelFunc, 0)
	for _, entry := range c.agents {
		if entry.status.ParentID != parentID || entry.status.State.Terminal() || entry.status.State == StateCanceling {
			continue
		}
		entry.status.State = StateCanceling
		cancels = append(cancels, entry.cancel)
	}
	c.agentsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return len(cancels)
}

func (c *Coordinator) Cancel(id string) error {
	c.agentsMu.Lock()
	entry := c.agents[strings.TrimSpace(id)]
	if entry == nil {
		c.agentsMu.Unlock()
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	if entry.status.State.Terminal() {
		c.agentsMu.Unlock()
		return nil
	}
	entry.status.State = StateCanceling
	cancel := entry.cancel
	c.agentsMu.Unlock()
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

// Close cancels all non-terminal subagents and waits for workers to exit.
