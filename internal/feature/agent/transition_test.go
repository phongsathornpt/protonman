package agent

import (
	"errors"
	"testing"
	"time"
)

func TestLifecycleStateTransitions(t *testing.T) {
	valid := [][2]State{
		{StateQueued, StateRunning}, {StateQueued, StateFailed}, {StateQueued, StateCanceled},
		{StateRunning, StateCompleted}, {StateRunning, StateFailed}, {StateRunning, StateCanceled},
		{StateRunning, StateCanceling}, {StateCanceling, StateCanceled}, {StateCanceling, StateCompleted},
		{StateQueued, StateInterrupted}, {StateRunning, StateInterrupted}, {StateCanceling, StateInterrupted},
		{StateInterrupted, StateResuming}, {StateResuming, StateResumed}, {StateResuming, StateInterrupted},
	}
	for _, edge := range valid {
		if !validStateTransition(edge[0], edge[1]) {
			t.Fatalf("expected valid transition %s -> %s", edge[0], edge[1])
		}
	}
}
func TestLifecycleStateTransitionRejectsInvalidEdge(t *testing.T) {
	status := AgentStatus{ID: "a-1", State: StateCompleted}
	got, err := transitionStatus(status, StateRunning, time.Now(), "")
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("transition error = %v, want ErrInvalidTransition", err)
	}
	if got.State != StateCompleted {
		t.Fatalf("invalid transition mutated state to %s", got.State)
	}
}

func TestTransitionStatusOwnsLifecycleTimestamps(t *testing.T) {
	started := time.Unix(100, 0)
	status, err := transitionStatus(AgentStatus{State: StateQueued}, StateRunning, started, "")
	if err != nil || !status.StartedAt.Equal(started) {
		t.Fatalf("running transition = %#v, %v", status, err)
	}
	finished := started.Add(time.Second)
	status, err = transitionStatus(status, StateCompleted, finished, "")
	if err != nil || !status.FinishedAt.Equal(finished) {
		t.Fatalf("completed transition = %#v, %v", status, err)
	}
}
