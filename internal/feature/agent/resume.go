package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const resumeSafetyContext = "This delegated task is resuming after a prior process interruption. The previous run may have partially changed workspace state. Re-inspect current state before making changes and do not assume earlier steps either succeeded or failed."

// Resume starts at most one fresh child from an interrupted durable record.
// A completed resume is idempotent: later retries return the same child handle.
func (c *Coordinator) Resume(ctx context.Context, id, parentID string) (Handle, error) {
	if c == nil {
		return Handle{}, ErrCoordinatorClosed
	}
	id = strings.TrimSpace(id)
	parentID = strings.TrimSpace(parentID)

	c.agentsMu.Lock()
	entry := c.agents[id]
	if entry == nil {
		c.agentsMu.Unlock()
		return Handle{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	if entry.status.State == StateResumed && entry.status.ResumedAs != "" {
		handle, ok := c.handleForIDLocked(entry.status.ResumedAs)
		c.agentsMu.Unlock()
		if ok {
			return handle, nil
		}
		return Handle{}, fmt.Errorf("%w: resumed child %q is unavailable", ErrNotFound, entry.status.ResumedAs)
	}
	if entry.status.State != StateInterrupted {
		state := entry.status.State
		c.agentsMu.Unlock()
		return Handle{}, fmt.Errorf("%w: %q is %s", ErrNotResumable, id, state)
	}
	status, err := transitionStatus(entry.status, StateResuming, time.Now(), "resume requested")
	if err != nil {
		c.agentsMu.Unlock()
		return Handle{}, err
	}
	entry.status = status
	req := entry.request
	c.agentsMu.Unlock()

	req.ID = ""
	req.ParentID = parentID
	req.ResumedFrom = id
	if strings.TrimSpace(req.Context) == "" {
		req.Context = resumeSafetyContext
	} else {
		req.Context = resumeSafetyContext + "\n\nPrevious task context:\n" + req.Context
	}
	handle, err := c.Spawn(ctx, req)
	if err != nil {
		c.rollbackResume(id)
		return Handle{}, err
	}
	c.agentsMu.Lock()
	source := c.agents[id]
	if source == nil {
		c.agentsMu.Unlock()
		_ = c.Cancel(handle.ID)
		return Handle{}, fmt.Errorf("%w: source %q disappeared during resume", ErrNotFound, id)
	}
	status, err = transitionStatus(source.status, StateResumed, time.Now(), "resumed as "+handle.ID)
	if err != nil {
		c.agentsMu.Unlock()
		_ = c.Cancel(handle.ID)
		return Handle{}, err
	}
	status.ResumedAs = handle.ID
	source.status = status
	c.agentsMu.Unlock()

	c.observeMetric(ctx, MetricEvent{Kind: MetricResumed, SessionID: req.SessionID, AgentID: handle.ID, ParentID: req.ParentID, Profile: handle.Profile})
	return handle, nil
}

func (c *Coordinator) rollbackResume(id string) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	entry := c.agents[id]
	if entry == nil || entry.status.State != StateResuming {
		return
	}
	status, err := transitionStatus(entry.status, StateInterrupted, time.Now(), "resume admission failed")
	if err == nil {
		entry.status = status
	}
}
func (c *Coordinator) handleForIDLocked(id string) (Handle, bool) {
	entry := c.agents[strings.TrimSpace(id)]
	if entry == nil {
		return Handle{}, false
	}
	return Handle{SessionID: entry.status.SessionID, ID: entry.status.ID, Profile: entry.status.Profile}, true
}
