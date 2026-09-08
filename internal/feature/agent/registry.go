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

// WaitActivity waits for the next terminal subagent mailbox activity across all parents.
// Observation timeout is non-fatal and never mutates child state.
func (c *Coordinator) WaitActivity(ctx context.Context, timeout time.Duration) (ActivityWaitResult, error) {
	return c.waitActivity(ctx, "", timeout)
}

// WaitActivityForParent waits for terminal activity owned by parentID only.
// Unrelated background agents cannot wake this wait.
func (c *Coordinator) WaitActivityForParent(ctx context.Context, parentID string, timeout time.Duration) (ActivityWaitResult, error) {
	return c.waitActivity(ctx, strings.TrimSpace(parentID), timeout)
}

func (c *Coordinator) waitActivity(ctx context.Context, parentID string, timeout time.Duration) (ActivityWaitResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = c.waitTimeout
	}

	consume := func() (*Event, <-chan struct{}) {
		c.activityMu.Lock()
		defer c.activityMu.Unlock()
		mailbox := c.activityMailboxes[parentID]
		if mailbox == nil {
			mailbox = &activityMailbox{notify: make(chan struct{})}
			c.activityMailboxes[parentID] = mailbox
		}
		if mailbox.seq > mailbox.seen {
			mailbox.seen = mailbox.seq
			ev := mailbox.event
			return &ev, nil
		}
		return nil, mailbox.notify
	}

	snapshot := func() []AgentStatus {
		agents := c.List()
		if parentID == "" {
			return agents
		}
		filtered := make([]AgentStatus, 0, len(agents))
		for _, status := range agents {
			if status.ParentID == parentID {
				filtered = append(filtered, status)
			}
		}
		return filtered
	}

	if ev, notify := consume(); ev != nil {
		return ActivityWaitResult{Event: ev, Agents: snapshot()}, nil
	} else {
		waitCtx := ctx
		cancel := func() {}
		if timeout > 0 {
			waitCtx, cancel = context.WithTimeout(ctx, timeout)
		}
		defer cancel()
		select {
		case <-notify:
			ev, _ := consume()
			return ActivityWaitResult{Event: ev, Agents: snapshot()}, nil
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
				c.observeMetric(ctx, MetricEvent{Kind: MetricWaitTimeout, ParentID: parentID})
				return ActivityWaitResult{Agents: snapshot(), TimedOut: true}, nil
			}
			return ActivityWaitResult{}, waitCtx.Err()
		}
	}
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
	return entry.status, true
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
	status := entry.status
	if !status.State.Terminal() {
		return status, nil, true
	}
	res := entry.result
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
			out = append(out, entry.status)
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
