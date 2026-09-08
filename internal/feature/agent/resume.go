package agent

import (
	"context"
	"fmt"
	"strings"
)

const resumeSafetyContext = "This delegated task is resuming after a prior process interruption. The previous run may have partially changed workspace state. Re-inspect current state before making changes and do not assume earlier steps either succeeded or failed."

// Resume explicitly starts a fresh child from an interrupted durable record.
// The interrupted record is retained; the resumed run receives a new agent ID.
func (c *Coordinator) Resume(ctx context.Context, id, parentID string) (Handle, error) {
	if c == nil {
		return Handle{}, ErrCoordinatorClosed
	}
	c.agentsMu.RLock()
	entry := c.agents[strings.TrimSpace(id)]
	if entry == nil {
		c.agentsMu.RUnlock()
		return Handle{}, fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	if entry.status.State != StateInterrupted {
		state := entry.status.State
		c.agentsMu.RUnlock()
		return Handle{}, fmt.Errorf("%w: %q is %s", ErrNotResumable, id, state)
	}
	req := entry.request
	c.agentsMu.RUnlock()

	req.ID = ""
	req.ParentID = strings.TrimSpace(parentID)
	if strings.TrimSpace(req.Context) == "" {
		req.Context = resumeSafetyContext
	} else {
		req.Context = resumeSafetyContext + "\n\nPrevious task context:\n" + req.Context
	}
	handle, err := c.Spawn(ctx, req)
	if err != nil {
		return Handle{}, err
	}
	c.observeMetric(ctx, MetricEvent{Kind: MetricResumed, SessionID: req.SessionID, AgentID: handle.ID, ParentID: req.ParentID, Profile: handle.Profile})
	return handle, nil
}
