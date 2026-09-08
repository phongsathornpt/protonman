package agent

import (
	"context"
	"strings"
)

// AgentRef is the stable identity of one subagent within its owning session.
type AgentRef struct {
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
}

// TurnRef identifies one parent turn within a session.
type TurnRef struct {
	SessionID string `json:"session_id"`
	TurnID    string `json:"turn_id"`
}

func (r AgentRef) normalized() AgentRef {
	r.SessionID = strings.TrimSpace(r.SessionID)
	r.AgentID = strings.TrimSpace(r.AgentID)
	return r
}

func (r TurnRef) normalized() TurnRef {
	r.SessionID = strings.TrimSpace(r.SessionID)
	r.TurnID = strings.TrimSpace(r.TurnID)
	return r
}

type turnRefContextKey struct{}

// WithTurnRef binds the owning session and parent turn to orchestration calls.
func WithTurnRef(ctx context.Context, ref TurnRef) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ref = ref.normalized()
	if ref.SessionID == "" && ref.TurnID == "" {
		return ctx
	}
	return context.WithValue(ctx, turnRefContextKey{}, ref)
}

// TurnRefFromContext returns the current orchestration scope, when present.
func TurnRefFromContext(ctx context.Context) TurnRef {
	if ctx == nil {
		return TurnRef{}
	}
	ref, _ := ctx.Value(turnRefContextKey{}).(TurnRef)
	return ref.normalized()
}

// SessionIDFromContext returns the owning session identifier, when present.
func SessionIDFromContext(ctx context.Context) string {
	return TurnRefFromContext(ctx).SessionID
}
