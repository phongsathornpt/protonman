package agent

import (
	"context"
	"strings"
)

// WithParentID is the compatibility wrapper for callers that only know a turn id.
// Existing session scope is preserved when one is already present on ctx.
func WithParentID(ctx context.Context, parentID string) context.Context {
	ref := TurnRefFromContext(ctx)
	ref.TurnID = strings.TrimSpace(parentID)
	return WithTurnRef(ctx, ref)
}

// ParentIDFromContext returns the owning parent-turn identifier, when present.
func ParentIDFromContext(ctx context.Context) string {
	return TurnRefFromContext(ctx).TurnID
}
