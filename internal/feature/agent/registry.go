package agent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Wait waits for a subagent for at most timeout. A wait timeout never cancels
// the child; it returns the current state so callers can wait again later.
func (c *Coordinator) Wait(ctx context.Context, id string, timeout time.Duration) (WaitResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.agentsMu.RLock()
	entry := c.agents[strings.TrimSpace(id)]
	if entry == nil {
		c.agentsMu.RUnlock()
		return WaitResult{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	done := entry.done
	state := entry.status.State
	c.agentsMu.RUnlock()
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
			wr, snapshotErr := c.waitSnapshot(id)
			wr.TimedOut = true
			if status, ok := c.Get(id); ok {
				c.observeMetric(ctx, MetricEvent{Kind: MetricWaitTimeout, AgentID: status.ID, ParentID: status.ParentID, Profile: status.Profile})
			}
			return wr, snapshotErr
		}
		return WaitResult{}, waitCtx.Err()
	}
}

// WaitActivity waits for the next terminal subagent lifecycle activity across all parents.
// Observation timeout is non-fatal and never mutates child state.
func (c *Coordinator) WaitActivity(ctx context.Context, timeout time.Duration) (ActivityWaitResult, error) {
	return c.waitActivity(ctx, TurnRef{}, nil, timeout)
}

// WaitActivityForTurn consumes the next ordered activity batch for one turn.
func (c *Coordinator) WaitActivityForTurn(ctx context.Context, ref TurnRef, timeout time.Duration) (ActivityWaitResult, error) {
	return c.waitActivity(ctx, ref.normalized(), nil, timeout)
}

// WaitActivityAfter reads activity after an explicit cursor without advancing the
// compatibility cursor. Independent waiters can therefore observe the same stream.
func (c *Coordinator) WaitActivityAfter(ctx context.Context, ref TurnRef, after uint64, timeout time.Duration) (ActivityWaitResult, error) {
	return c.waitActivity(ctx, ref.normalized(), &after, timeout)
}

func activityScopeKey(ref TurnRef) string {
	ref = ref.normalized()
	if ref.SessionID == "" && ref.TurnID == "" {
		return ""
	}
	return ref.SessionID + "\x00" + ref.TurnID
}

func (c *Coordinator) waitActivity(ctx context.Context, ref TurnRef, after *uint64, timeout time.Duration) (ActivityWaitResult, error) {
	defer c.pruneActivityMailboxes(time.Now())
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = c.waitTimeout
	}

	consume := func() (ActivityWaitResult, <-chan struct{}) {
		c.activityMu.Lock()
		defer c.activityMu.Unlock()
		scope := activityScopeKey(ref)
		mailbox := c.activityMailboxes[scope]
		if mailbox == nil {
			mailbox = &activityMailbox{notify: make(chan struct{}), updatedAt: time.Now()}
			c.activityMailboxes[scope] = mailbox
		}
		mailbox.updatedAt = time.Now()
		cursor := mailbox.seen
		if after != nil {
			cursor = *after
		}
		events, nextCursor, truncated := activityEventsAfter(mailbox, cursor)
		if len(events) == 0 {
			return ActivityWaitResult{Events: []Event{}, Cursor: cursor}, mailbox.notify
		}
		if after == nil {
			mailbox.seen = nextCursor
		}
		last := events[len(events)-1]
		return ActivityWaitResult{Event: &last, Events: events, Cursor: nextCursor, Truncated: truncated}, nil
	}

	if result, notify := consume(); len(result.Events) > 0 {
		c.attachActivitySnapshot(ctx, ref, &result)
		return result, nil
	} else {
		waitCtx := ctx
		cancel := func() {}
		if timeout > 0 {
			waitCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		defer cancel()
		select {
		case <-notify:
			result, _ := consume()
			c.attachActivitySnapshot(ctx, ref, &result)
			return result, nil
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				c.observeMetric(ctx, MetricEvent{Kind: MetricWaitTimeout, SessionID: ref.SessionID, ParentID: ref.TurnID})
				c.attachActivitySnapshot(ctx, ref, &result)
				result.TimedOut = true
				return result, nil
			}
			return ActivityWaitResult{}, waitCtx.Err()
		}
	}
}

func (c *Coordinator) attachActivitySnapshot(ctx context.Context, ref TurnRef, result *ActivityWaitResult) {
	if result == nil {
		return
	}
	result.Agents = c.activitySnapshot(ref)
	c.observeMetric(ctx, MetricEvent{
		Kind: MetricWaitSnapshotBytes, SessionID: ref.SessionID, ParentID: ref.TurnID,
		Bytes: metricJSONBytes(result.Agents), Count: len(result.Agents),
	})
}

func activityEventsAfter(mailbox *activityMailbox, after uint64) ([]Event, uint64, bool) {
	if mailbox == nil || len(mailbox.events) == 0 {
		return []Event{}, after, false
	}
	oldest := mailbox.events[0].seq
	truncated := after+1 < oldest
	events := make([]Event, 0, len(mailbox.events))
	next := after
	for _, record := range mailbox.events {
		if record.seq <= after {
			continue
		}
		events = append(events, record.event)
		next = record.seq
	}
	return events, next, truncated
}

func (c *Coordinator) activitySnapshot(ref TurnRef) []AgentStatus {
	agents := c.List()
	if ref.SessionID == "" && ref.TurnID == "" {
		return agents
	}
	filtered := make([]AgentStatus, 0, len(agents))
	for _, status := range agents {
		if ref.SessionID != "" && status.SessionID != ref.SessionID {
			continue
		}
		if ref.TurnID != "" && status.ParentID != ref.TurnID {
			continue
		}
		filtered = append(filtered, status)
	}
	return filtered
}

func (c *Coordinator) waitSnapshot(id string) (WaitResult, error) {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	entry := c.agents[id]
	if entry == nil {
		return WaitResult{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	wr := WaitResult{State: entry.status.State}
	if entry.status.State.Terminal() {
		res := entry.result
		if c.resultStore != nil {
			if stored, ok := c.resultStore.Get(entry.resultRef); ok {
				res = stored
			}
		}
		wr.Result = &res
	}
	return wr, nil
}

func (c *Coordinator) pruneExpired() {
	if c.resultTTL <= 0 && c.maxRetainedAgents <= 0 {
		return
	}
	c.agentsMu.Lock()
	c.pruneExpiredLocked(time.Now())
	c.agentsMu.Unlock()
}

func (c *Coordinator) pruneExpiredLocked(now time.Time) {
	if c.resultTTL > 0 {
		for id, entry := range c.agents {
			if entry.status.State.Terminal() && !entry.status.FinishedAt.IsZero() && now.Sub(entry.status.FinishedAt) >= c.resultTTL {
				if c.resultStore != nil {
					c.resultStore.Delete(entry.resultRef)
				}
				delete(c.agents, id)
			}
		}
	}
	if c.maxRetainedAgents <= 0 {
		return
	}
	type retained struct {
		id       string
		finished time.Time
	}
	terminal := make([]retained, 0)
	for id, entry := range c.agents {
		if entry.status.State.Terminal() {
			terminal = append(terminal, retained{id: id, finished: entry.status.FinishedAt})
		}
	}
	if len(terminal) <= c.maxRetainedAgents {
		return
	}
	sort.Slice(terminal, func(i, j int) bool {
		if terminal[i].finished.Equal(terminal[j].finished) {
			return terminal[i].id < terminal[j].id
		}
		return terminal[i].finished.Before(terminal[j].finished)
	})
	for _, item := range terminal[:len(terminal)-c.maxRetainedAgents] {
		if entry := c.agents[item.id]; entry != nil && c.resultStore != nil {
			c.resultStore.Delete(entry.resultRef)
		}
		delete(c.agents, item.id)
	}
}

// Get returns one retained subagent status.
func (c *Coordinator) Get(id string) (AgentStatus, bool) {
	c.pruneExpired()
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	entry, ok := c.agents[strings.TrimSpace(id)]
	if !ok {
		return AgentStatus{}, false
	}
	return cloneAgentStatus(entry.status), true
}

// Lookup returns one retained status and, when terminal, its result.
func (c *Coordinator) Lookup(id string) (AgentStatus, *Result, bool) {
	c.pruneExpired()
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	entry, ok := c.agents[strings.TrimSpace(id)]
	if !ok {
		return AgentStatus{}, nil, false
	}
	status := cloneAgentStatus(entry.status)
	if !status.State.Terminal() {
		return status, nil, true
	}
	res := entry.result
	if c.resultStore != nil {
		if stored, ok := c.resultStore.Get(entry.resultRef); ok {
			res = stored
		}
	}
	return status, &res, true
}

// Active returns queued, running, or canceling subagents only.
func (c *Coordinator) Active() []AgentStatus {
	c.pruneExpired()
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	out := make([]AgentStatus, 0, len(c.agents))
	for _, entry := range c.agents {
		if !entry.status.State.Terminal() {
			out = append(out, cloneAgentStatus(entry.status))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartTime.Equal(out[j].StartTime) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartTime.Before(out[j].StartTime)
	})
	return out
}

// List returns all retained subagents, including terminal results.
func (c *Coordinator) List() []AgentStatus {
	c.pruneExpired()
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	out := make([]AgentStatus, 0, len(c.agents))
	for _, entry := range c.agents {
		out = append(out, cloneAgentStatus(entry.status))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartTime.Equal(out[j].StartTime) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartTime.Before(out[j].StartTime)
	})
	return out
}
