package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (c *Coordinator) persistLifecycleEvent(ctx context.Context, event LifecycleEvent) error {
	if c == nil || c.lifecycleStore == nil {
		return nil
	}
	if strings.TrimSpace(event.SessionID) == "" {
		return fmt.Errorf("persist lifecycle event %s: session id is required", event.Kind)
	}
	if ctx == nil {
		ctx = c.rootCtx
	}
	if err := c.lifecycleStore.AppendLifecycleEvent(ctx, event); err != nil {
		return fmt.Errorf("persist lifecycle event %s for %s: %w", event.Kind, event.AgentID, err)
	}
	return nil
}

func (c *Coordinator) persistAndApplyEntry(ctx context.Context, entry *agentEntry, event LifecycleEvent) error {
	if err := c.persistLifecycleEvent(ctx, event); err != nil {
		return err
	}
	return applyEntryLifecycleEvent(entry, event)
}
func (c *Coordinator) persistAndApplyTransition(
	ctx context.Context,
	entry *agentEntry,
	kind LifecycleEventKind,
	at time.Time,
	reason string,
) error {
	event := nextLifecycleEvent(entry.status, kind, at, reason)
	return c.persistAndApplyEntry(ctx, entry, event)
}

// CompactLifecycleSession installs a session-scoped projection snapshot and
// discards lifecycle facts already represented by that snapshot. The agent lock
// prevents a concurrent append from slipping between snapshot capture and journal truncation.
func (c *Coordinator) CompactLifecycleSession(ctx context.Context, sessionID string) error {
	if c == nil || c.lifecycleStore == nil {
		return nil
	}
	compactor, ok := c.lifecycleStore.(LifecycleEventCompactor)
	if !ok {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("compact lifecycle: session id is required")
	}
	if ctx == nil {
		ctx = c.rootCtx
	}
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	if c.closed.Load() {
		return ErrCoordinatorClosed
	}
	snapshot := c.persistentSnapshotLocked(sessionID)
	if err := compactor.CompactLifecycle(ctx, sessionID, snapshot); err != nil {
		return fmt.Errorf("compact lifecycle for session %s: %w", sessionID, err)
	}
	return nil
}
