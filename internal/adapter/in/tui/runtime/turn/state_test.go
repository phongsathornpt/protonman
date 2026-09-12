package turn

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestTurnStateLifecycle(t *testing.T) {
	started := time.Unix(123, 0)
	state := State{}
	state.BeginTurn("turn-1", started)
	if !state.Busy || state.ActiveTurnOwner != "turn-1" || state.Activity != "" || !state.BusyStarted.Equal(started) {
		t.Fatalf("started state = %+v", state)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan tea.Msg)
	state.BindTurn(cancel, events)
	if state.Cancel == nil || state.Events == nil {
		t.Fatal("turn runtime handles were not bound")
	}
	state.FinishTurn()
	if state.Busy || state.Cancel != nil || state.Events != nil || state.ActiveTurnOwner != "" || state.Activity != "ready" {
		t.Fatalf("finished state = %+v", state)
	}
	_ = ctx
}

func TestTurnStateToolLifecycle(t *testing.T) {
	started := time.Unix(456, 0)
	state := State{}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	state.BeginTool("running read", started, cancel)
	if !state.Busy || state.Activity != "running read" || state.Cancel == nil {
		t.Fatalf("tool state = %+v", state)
	}
	state.FinishTool()
	if state.Busy || state.Activity != "ready" || state.Cancel != nil {
		t.Fatalf("finished tool state = %+v", state)
	}
}
