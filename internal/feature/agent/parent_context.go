package agent

import "context"

// ParentIDFromContext returns the owning parent-turn identifier, when present.
func ParentIDFromContext(ctx context.Context) string {
	return TurnRefFromContext(ctx).TurnID
}
