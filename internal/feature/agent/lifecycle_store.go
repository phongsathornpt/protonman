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
