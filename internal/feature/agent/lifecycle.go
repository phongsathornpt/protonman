package agent

import (
	"context"
	"errors"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/base/contextutil"
	"github.com/phongsathornpt/protonman/internal/base/failure"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func (c *Coordinator) Spawn(ctx context.Context, req Request) (Handle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Handle{}, err
	}
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.ParentID = strings.TrimSpace(req.ParentID)
	if err := req.Validate(); err != nil {
		return Handle{}, fmt.Errorf("invalid subagent request: %w", err)
	}
	c.agentsMu.Lock()
	if !c.enabled.Load() {
		c.agentsMu.Unlock()
		return Handle{}, ErrSubagentsDisabled
	}
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
	boundModel := c.languageModel
	if c.modelResolver != nil {
		boundModel = c.modelResolver.Resolve(req.Profile, boundModel)
	}
	providerName, modelID := languageModelIdentity(boundModel)
	boundReasoning := c.reasoningEffort
	if c.reasoningResolver != nil {
		boundReasoning = c.reasoningResolver.Resolve(req.Profile, boundReasoning)
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
	queuedEvent := LifecycleEvent{
		Kind: LifecycleAgentQueued, Version: 1, At: queuedAt, SessionID: req.SessionID, ParentID: req.ParentID,
		AgentID: id, Profile: req.Profile, Task: req.Task, Optional: req.Optional, Provider: providerName, Model: modelID, ResumedFrom: req.ResumedFrom,
		Request: &req,
	}
	if err := c.persistLifecycleEvent(ctx, queuedEvent); err != nil {
		c.agentsMu.Unlock()
		runCancel()
		return Handle{}, err
	}
	queuedStatus, transitionErr := applyLifecycleEvent(AgentStatus{}, queuedEvent)
	if transitionErr != nil {
		c.agentsMu.Unlock()
		runCancel()
		return Handle{}, transitionErr
	}
	runtimeSpec := childToolRuntime{
		permissionMode: c.permissionMode, prompt: c.prompt, guard: c.guard,
		permissionTimeout: c.toolPermissionTimeout, executionTimeout: c.toolExecutionTimeout, observer: c.toolObserver,
	}
	entry := &agentEntry{
		request: req, languageModel: boundModel, reasoningEffort: boundReasoning, toolRuntime: runtimeSpec, status: queuedStatus,
		cancel: runCancel, done: make(chan struct{}), started: make(chan struct{}),
	}
	c.agents[id] = entry
	// Add while admission is still serialized with Close so Wait can never
	// observe a zero counter for a child that has already been admitted.
	c.wg.Add(1)
	c.agentsMu.Unlock()

	c.emit(runCtx, Event{Kind: EventAgentQueued, SessionID: req.SessionID, AgentID: id, ParentID: req.ParentID, Profile: req.Profile, Message: req.Task})
	go c.runEntry(runCtx, entry, req, queuedAt)
	return Handle{SessionID: req.SessionID, ID: id, Profile: req.Profile}, nil
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
		err := queueCtx.Err()
		if errors.Is(err, context.DeadlineExceeded) && runCtx.Err() == nil {
			err = queueTimeoutError()
		}
		c.finishEntry(entry, req, queuedAt, time.Time{}, err)
		return
	}
	defer func() { <-c.sem }()

	releaseWorkspace, err := c.acquireWorkspace(queueCtx, req.Profile.IsMutating())
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) && runCtx.Err() == nil {
			err = queueTimeoutError()
		}
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
	if transitionErr := c.persistAndApplyTransition(runCtx, entry, LifecycleAgentStarted, startedAt, ""); transitionErr != nil {
		c.agentsMu.Unlock()
		c.finishEntry(entry, req, queuedAt, time.Time{}, transitionErr)
		return
	}
	close(entry.started)
	c.agentsMu.Unlock()
	queueDuration := startedAt.Sub(queuedAt)

	execCtx := runCtx
	execCancel := func() {}
	if executionTimeout > 0 {
		execCtx, execCancel = context.WithTimeout(runCtx, executionTimeout)
	}
	defer execCancel()

	c.emit(execCtx, Event{Kind: EventAgentStarted, SessionID: req.SessionID, AgentID: req.ID, ParentID: req.ParentID, Profile: req.Profile, Message: req.Task, QueueDuration: queueDuration})
	res, runErr := c.executeWithRuntime(execCtx, req, entry.languageModel, entry.reasoningEffort, entry.toolRuntime)
	if errors.Is(runErr, context.DeadlineExceeded) && runCtx.Err() == nil && execCtx.Err() != nil {
		runErr = executionTimeoutError()
	}
	c.agentsMu.RLock()
	provider, modelID := entry.status.Provider, entry.status.Model
	c.agentsMu.RUnlock()
	res.Provider = provider
	res.Model = modelID
	res.QueueDuration = queueDuration
	res.Duration = time.Since(startedAt)
	res.TotalDuration = time.Since(queuedAt)
	if runErr != nil {
		res.Err = runErr
	}
	if transitionErr := c.storeTerminal(c.rootCtx, entry, res, runErr); transitionErr != nil {
		runErr = transitionErr
		res.Err = transitionErr
	}

	c.emitStoredResult(execCtx, entry, req, res)

	eventKind := EventAgentCompleted
	if runErr != nil {
		eventKind = EventAgentFailed
	}
	emitCtx, emitDone := contextutil.DetachedTimeout(execCtx, runtimepolicy.AgentLifecycleEmitTimeout)
	c.emit(emitCtx, Event{Kind: eventKind, SessionID: req.SessionID, AgentID: req.ID, ParentID: req.ParentID, Profile: req.Profile, Message: res.Summary, QueueDuration: res.QueueDuration, Duration: res.Duration, TotalDuration: res.TotalDuration, Err: runErr})
	emitDone()
}

func (c *Coordinator) finishEntry(entry *agentEntry, req Request, queuedAt, startedAt time.Time, err error) {
	now := time.Now()
	res := Result{SessionID: req.SessionID, AgentID: req.ID, Profile: req.Profile, Provider: entry.status.Provider, Model: entry.status.Model, QueueDuration: now.Sub(queuedAt), TotalDuration: now.Sub(queuedAt), Err: err}
	if !startedAt.IsZero() {
		res.Duration = now.Sub(startedAt)
	}
	if transitionErr := c.storeTerminal(c.rootCtx, entry, res, err); transitionErr != nil {
		err = transitionErr
		res.Err = transitionErr
	}
	c.emitStoredResult(c.rootCtx, entry, req, res)
	c.emit(c.rootCtx, Event{Kind: EventAgentFailed, SessionID: req.SessionID, AgentID: req.ID, ParentID: req.ParentID, Profile: req.Profile, QueueDuration: res.QueueDuration, Duration: res.Duration, TotalDuration: res.TotalDuration, Err: err})
	if startedAt.IsZero() {
		close(entry.started)
	}
}

func (c *Coordinator) emitStoredResult(ctx context.Context, entry *agentEntry, req Request, res Result) {
	c.agentsMu.RLock()
	version := entry.resultRef.Version
	c.agentsMu.RUnlock()
	if version == 0 {
		return
	}
	c.emit(ctx, Event{
		Kind: EventAgentResultAvailable, SessionID: req.SessionID, AgentID: req.ID, ParentID: req.ParentID,
		Profile: req.Profile, ResultVersion: version, QueueDuration: res.QueueDuration,
		Duration: res.Duration, TotalDuration: res.TotalDuration,
	})
}

func (c *Coordinator) storeTerminal(ctx context.Context, entry *agentEntry, res Result, err error) error {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	kind := LifecycleAgentFailed
	switch {
	case err == nil:
		kind = LifecycleAgentCompleted
	case errors.Is(err, context.Canceled):
		kind = LifecycleAgentCanceled
	}
	event := nextLifecycleEvent(entry.status, kind, time.Now(), terminalReason(err))
	resultCopy := compactRetainedResult(res)
	resultCopy.Err = nil
	event.Result = &resultCopy
	if err != nil {
		event.Error = err.Error()
	}
	if transitionErr := c.persistAndApplyEntry(ctx, entry, event); transitionErr != nil {
		return transitionErr
	}
	entry.result = compactRetainedResult(res)
	entry.err = err
	entry.resultRef = ResultRef{SessionID: event.SessionID, AgentID: event.AgentID, Version: event.Version}
	if c.resultStore != nil {
		c.resultStore.Put(entry.resultRef, entry.result)
	}
	return nil
}

func languageModelIdentity(languageModel sdk.LanguageModel) (provider, modelID string) {
	if languageModel == nil {
		return "", ""
	}
	return strings.TrimSpace(languageModel.Provider()), strings.TrimSpace(languageModel.ModelID())
}

func terminalReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrQueueTimeout):
		return "queue timed out"
	case errors.Is(err, ErrExecutionTimeout):
		return "execution timed out"
	case errors.Is(err, context.DeadlineExceeded):
		return "timed out"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, ErrUnverifiedChanges):
		return "unverified changes"
	case errors.Is(err, toolcall.ErrPermissionDenied):
		return "permission denied"
	}
	if classified, ok := failure.ClassifyProvider(err); ok {
		message := failure.Summary(classified.Code)
		if classified.Code == failure.CodeModelInvalidRequest && classified.Message != "" {
			message += ": " + classified.Message
		}
		return truncateTerminalReason(message)
	}
	return truncateTerminalReason(err.Error())
}

func truncateTerminalReason(value string) string {
	msg := strings.ToValidUTF8(strings.TrimSpace(value), "�")
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
		if err := c.persistAndApplyTransition(c.rootCtx, entry, LifecycleAgentCancelRequested, time.Now(), "cancel requested"); err != nil {
			continue
		}
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
	if err := c.persistAndApplyTransition(c.rootCtx, entry, LifecycleAgentCancelRequested, time.Now(), "cancel requested"); err != nil {
		c.agentsMu.Unlock()
		return err
	}
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
		return Result{SessionID: h.SessionID, AgentID: h.ID, Profile: h.Profile, Err: ctx.Err()}, ctx.Err()
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
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			err = executionTimeoutError()
		}
		return Result{SessionID: h.SessionID, AgentID: h.ID, Profile: h.Profile, Err: err}, err
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
