package agent

import (
	"context"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

func TestOptionalChildDoesNotBlockParentCompletion(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxConcurrency(2),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				<-ctx.Done()
				return turn.Result{}, ctx.Err()
			}}, nil
		}),
	)
	defer coord.Close()
	ref := TurnRef{SessionID: "session-a", TurnID: "turn-a"}
	required, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "required"})
	if err != nil {
		t.Fatal(err)
	}
	optional, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "speculative", Optional: true})
	if err != nil {
		t.Fatal(err)
	}
	if !coord.HasLiveForTurn(ref) || !coord.HasBlockingLiveForTurn(ref) {
		t.Fatal("required child should keep both live and completion-barrier state active")
	}
	if got := coord.CancelOptionalByTurn(ref); got != 1 {
		t.Fatalf("CancelOptionalByTurn()=%d, want 1", got)
	}
	waitForTest(t, time.Second, func() bool {
		status, ok := coord.Get(optional.ID)
		return ok && status.State.Terminal()
	})
	if status, _ := coord.Get(required.ID); status.State.Terminal() || status.State == StateCanceling {
		t.Fatalf("required child was affected by optional cleanup: %+v", status)
	}
	if !coord.HasBlockingLiveForTurn(ref) {
		t.Fatal("required child stopped blocking after optional cleanup")
	}
	if got := coord.CancelByTurn(ref); got != 1 {
		t.Fatalf("CancelByTurn()=%d, want remaining required child", got)
	}
}

func TestOptionalOnlyTurnIsLiveButNotBlocking(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	ref := TurnRef{SessionID: "session-a", TurnID: "turn-a"}
	_, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "speculative", Optional: true})
	if err != nil {
		t.Fatal(err)
	}
	if !coord.HasLiveForTurn(ref) {
		t.Fatal("optional child should remain visible as live work")
	}
	if coord.HasBlockingLiveForTurn(ref) {
		t.Fatal("optional child must not hold the parent completion barrier")
	}
	coord.CancelOptionalByTurn(ref)
}
