package agent

import (
	"context"
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

// HasLiveForTurn reports whether one parent turn still owns non-terminal children.
func (c *Coordinator) HasLiveForTurn(ref TurnRef) bool {
	return c.hasLiveForTurn(ref, false)
}

// HasBlockingLiveForTurn reports whether the turn owns non-terminal children whose
// results are required before the parent may commit its final response. Optional
// speculative children are intentionally excluded from this completion barrier.
func (c *Coordinator) HasBlockingLiveForTurn(ref TurnRef) bool {
	return c.hasLiveForTurn(ref, true)
}

func (c *Coordinator) hasLiveForTurn(ref TurnRef, blockingOnly bool) bool {
	if c == nil {
		return false
	}
	ref = ref.normalized()
	c.pruneExpired()
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	for _, entry := range c.agents {
		if ref.SessionID != "" && entry.status.SessionID != ref.SessionID {
			continue
		}
		if ref.TurnID != "" && entry.status.ParentID != ref.TurnID {
			continue
		}
		if blockingOnly && entry.status.Optional {
			continue
		}
		if !entry.status.State.Terminal() {
			return true
		}
	}
	return false
}

// CancelOptionalByTurn cancels speculative children that are still live when the
// owning parent turn commits. Their completed results remain retained and any
// result that became ready before completion can still be integrated normally.
func (c *Coordinator) CancelOptionalByTurn(ref TurnRef) int {
	ref = ref.normalized()
	if ref.TurnID == "" {
		return 0
	}
	c.agentsMu.Lock()
	cancels := make([]func(), 0)
	for _, entry := range c.agents {
		if entry.status.SessionID != ref.SessionID || entry.status.ParentID != ref.TurnID || !entry.status.Optional || entry.status.State.Terminal() || entry.status.State == StateCanceling {
			continue
		}
		if err := c.persistAndApplyTransition(c.rootCtx, entry, LifecycleAgentCancelRequested, time.Now(), "optional child no longer needed after parent completion"); err != nil {
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
	if err := c.persistAndApplyTransition(c.rootCtx, entry, LifecycleAgentCancelRequested, time.Now(), "cancel requested"); err != nil {
		c.agentsMu.Unlock()
		return err
	}
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

// CancelPolicy controls whether canceling a root turn also cancels owned children.
type CancelPolicy uint8

const (
	CancelTurnOnly CancelPolicy = iota
	CancelTurnAndChildren
)

// CancelTurn applies the shared root-turn cancellation policy for all adapters.
func (c *Coordinator) CancelTurn(ref TurnRef, policy CancelPolicy) int {
	if policy != CancelTurnAndChildren {
		return 0
	}
	return c.CancelByTurn(ref)
}

// CancelSessionAndWait requests cancellation for every live child owned by one
// session and waits until their runtimes have stopped or ctx is canceled.
func (c *Coordinator) CancelSessionAndWait(ctx context.Context, sessionID string) (int, error) {
	if c == nil {
		return 0, ErrCoordinatorClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, fmt.Errorf("cancel session: session id is required")
	}
	c.agentsMu.Lock()
	cancels := make([]func(), 0)
	done := make([]<-chan struct{}, 0)
	count := 0
	for _, entry := range c.agents {
		if entry.status.SessionID != sessionID || entry.status.State.Terminal() {
			continue
		}
		done = append(done, entry.done)
		if entry.status.State != StateCanceling {
			if err := c.persistAndApplyTransition(ctx, entry, LifecycleAgentCancelRequested, time.Now(), "session canceled"); err != nil {
				c.agentsMu.Unlock()
				return count, err
			}
			count++
		}
		cancels = append(cancels, entry.cancel)
	}
	c.agentsMu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	for _, stopped := range done {
		select {
		case <-stopped:
		case <-ctx.Done():
			return count, ctx.Err()
		}
	}
	return count, nil
}
