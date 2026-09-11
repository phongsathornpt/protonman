package agent

import (
	"context"
	"fmt"
	"strings"
)

func normalizeDependencyIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (c *Coordinator) resolveDependenciesLocked(req Request) ([]*agentEntry, error) {
	if len(req.DependsOn) == 0 {
		return nil, nil
	}
	dependencies := make([]*agentEntry, 0, len(req.DependsOn))
	for _, id := range req.DependsOn {
		if id == req.ID {
			return nil, fmt.Errorf("%w: %q cannot depend on itself", ErrDependencyScope, id)
		}
		entry := c.agents[id]
		if entry == nil {
			return nil, fmt.Errorf("%w: %q", ErrDependencyNotFound, id)
		}
		if entry.status.SessionID != req.SessionID || entry.status.ParentID != req.ParentID {
			return nil, fmt.Errorf("%w: %q", ErrDependencyScope, id)
		}
		if entry.status.State.Terminal() && entry.status.State != StateCompleted {
			return nil, dependencyStateError(id, entry.status.State)
		}
		dependencies = append(dependencies, entry)
	}
	return dependencies, nil
}

func (c *Coordinator) waitDependencies(ctx context.Context, dependencies []*agentEntry) error {
	for _, dependency := range dependencies {
		if dependency == nil {
			continue
		}
		select {
		case <-dependency.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		c.agentsMu.RLock()
		id := dependency.status.ID
		state := dependency.status.State
		c.agentsMu.RUnlock()
		if state != StateCompleted {
			return dependencyStateError(id, state)
		}
	}
	return nil
}

func dependencyStateError(id string, state State) error {
	return fmt.Errorf("%w: %q is %s", ErrDependencyFailed, id, state)
}
