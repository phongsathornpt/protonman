package runtime

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestTurnStateLifecycle(t *testing.T) {
	started := time.Unix(123, 0)
	state := turnModelState{}
	state.beginTurn("turn-1", started)
	if !state.busy || state.activeTurnOwner != "turn-1" || state.activity != "" || !state.busyStarted.Equal(started) {
		t.Fatalf("started state = %+v", state)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan tea.Msg)
	state.bindTurn(cancel, events)
	if state.turnCancel == nil || state.turnEvents == nil {
		t.Fatal("turn runtime handles were not bound")
	}
	state.finishTurn()
	if state.busy || state.turnCancel != nil || state.turnEvents != nil || state.activeTurnOwner != "" || state.activity != "ready" {
		t.Fatalf("finished state = %+v", state)
	}
	_ = ctx
}
