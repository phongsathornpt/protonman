package agent

import (
	"context"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

func TestTurnRefContextPreservesSessionThroughParentCompatibility(t *testing.T) {
	ctx := WithTurnRef(context.Background(), TurnRef{SessionID: " session-a ", TurnID: " turn-1 "})
	ctx = WithParentID(ctx, "turn-2")
	got := TurnRefFromContext(ctx)
	if got != (TurnRef{SessionID: "session-a", TurnID: "turn-2"}) {
		t.Fatalf("TurnRefFromContext() = %#v", got)
	}
	if SessionIDFromContext(ctx) != "session-a" || ParentIDFromContext(ctx) != "turn-2" {
		t.Fatalf("compatibility accessors lost scope: %#v", got)
	}
}
func TestWaitActivityForTurnIsolatesSameTurnIDAcrossSessions(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return hardeningRunner{}, nil
		}),
	)
	defer coord.Close()

	for _, sessionID := range []string{"session-a", "session-b"} {
		h, err := coord.Spawn(context.Background(), Request{
			SessionID: sessionID, ParentID: "turn-1", Profile: ProfileAgility, Task: "inspect " + sessionID,
		})
		if err != nil {
			t.Fatalf("spawn %s: %v", sessionID, err)
		}
		if h.SessionID != sessionID {
			t.Fatalf("handle session = %q, want %q", h.SessionID, sessionID)
		}
	}

	for _, sessionID := range []string{"session-a", "session-b"} {
		got, err := coord.WaitActivityForTurn(context.Background(), TurnRef{SessionID: sessionID, TurnID: "turn-1"}, time.Second)
		if err != nil {
			t.Fatalf("wait %s: %v", sessionID, err)
		}
		if got.Event == nil || got.Event.SessionID != sessionID || got.Event.ParentID != "turn-1" {
			t.Fatalf("wait %s returned %#v", sessionID, got.Event)
		}
		if len(got.Agents) != 1 || got.Agents[0].SessionID != sessionID {
			t.Fatalf("wait %s leaked snapshot: %#v", sessionID, got.Agents)
		}
	}
}
