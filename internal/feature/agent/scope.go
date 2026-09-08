package agent

import (
	"fmt"
	"strings"
	"time"
)

// GetRef returns one subagent only when it belongs to the requested session.
func (c *Coordinator) GetRef(ref AgentRef) (AgentStatus, bool) {
	ref = ref.normalized()
	c.pruneExpired()
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	entry := c.agents[ref.AgentID]
	if entry == nil || entry.status.SessionID != ref.SessionID {
		return AgentStatus{}, false
	}
	return entry.status, true
}

// LookupRef returns one session-owned status and terminal result.
func (c *Coordinator) LookupRef(ref AgentRef) (AgentStatus, *Result, bool) {
	ref = ref.normalized()
	c.pruneExpired()
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	entry := c.agents[ref.AgentID]
	if entry == nil || entry.status.SessionID != ref.SessionID {
		return AgentStatus{}, nil, false
	}
	status := entry.status
	if !status.State.Terminal() {
		return status, nil, true
	}
	result := entry.result
	return status, &result, true
}

// ListSession returns retained subagents owned by one session.
func (c *Coordinator) ListSession(sessionID string) []AgentStatus {
	sessionID = strings.TrimSpace(sessionID)
	all := c.List()
	out := make([]AgentStatus, 0, len(all))
	for _, status := range all {
		if status.SessionID == sessionID {
			out = append(out, status)
		}
	}
	return out
}

// CancelRef requests cancellation only when the agent belongs to ref.SessionID.
func (c *Coordinator) CancelRef(ref AgentRef) error {
	ref = ref.normalized()
	c.agentsMu.Lock()
	entry := c.agents[ref.AgentID]
	if entry == nil || entry.status.SessionID != ref.SessionID {
		c.agentsMu.Unlock()
		return fmt.Errorf("%w: %q", ErrNotFound, ref.AgentID)
	}
	if entry.status.State.Terminal() {
		c.agentsMu.Unlock()
		return nil
	}
	status, err := transitionStatus(entry.status, StateCanceling, time.Now(), "cancel requested")
	if err != nil {
		c.agentsMu.Unlock()
		return err
	}
	entry.status = status
	cancel := entry.cancel
	c.agentsMu.Unlock()
	cancel()
	return nil
}

// CancelByTurn cancels non-terminal children owned by one session/turn pair.
func (c *Coordinator) CancelByTurn(ref TurnRef) int {
	ref = ref.normalized()
	if ref.TurnID == "" {
		return 0
	}
	c.agentsMu.Lock()
	cancels := make([]func(), 0)
	for _, entry := range c.agents {
		if entry.status.SessionID != ref.SessionID || entry.status.ParentID != ref.TurnID || entry.status.State.Terminal() || entry.status.State == StateCanceling {
			continue
		}
		status, err := transitionStatus(entry.status, StateCanceling, time.Now(), "cancel requested")
		if err != nil {
			continue
		}
		entry.status = status
		cancels = append(cancels, entry.cancel)
	}
	c.agentsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	return len(cancels)
}
