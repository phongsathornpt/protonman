package agent

import (
	"context"
	"strings"
)

type parentIDContextKey struct{}

// WithParentID associates delegated subagents with one owning root turn.
func WithParentID(ctx context.Context, parentID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return ctx
	}
	return context.WithValue(ctx, parentIDContextKey{}, parentID)
}

// ParentIDFromContext returns the owning root-turn identifier, when present.
func ParentIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	parentID, _ := ctx.Value(parentIDContextKey{}).(string)
	return strings.TrimSpace(parentID)
}
