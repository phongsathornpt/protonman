package agent

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

func newResumeTestCoordinator(t *testing.T) *Coordinator {
	t.Helper()
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) { return hardeningRunner{}, nil }),
	)
	if err := coord.RestorePersistentSnapshot(PersistentSnapshot{Version: PersistentSnapshotVersion, Agents: []PersistentAgent{{
		Status:  AgentStatus{ID: "strength-7", Profile: ProfileStrength, Task: "finish", State: StateRunning},
		Request: Request{ID: "strength-7", Profile: ProfileStrength, Task: "finish"},
	}}}); err != nil {
		t.Fatal(err)
	}
	return coord
}
func TestResumeIsIdempotentAfterSuccessfulAdmission(t *testing.T) {
	coord := newResumeTestCoordinator(t)
	defer coord.Close()

	first, err := coord.Resume(context.Background(), "strength-7", "turn-new")
	if err != nil {
		t.Fatal(err)
	}
	second, err := coord.Resume(context.Background(), "strength-7", "turn-new")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("second resume created %q, want %q", second.ID, first.ID)
	}
	if got := len(coord.List()); got != 2 {
		t.Fatalf("retained agents = %d, want source + one child", got)
	}
	source, ok := coord.Get("strength-7")
	if !ok || source.State != StateResumed || source.ResumedAs != first.ID {
		t.Fatalf("source after resume = %#v", source)
	}
	child, ok := coord.Get(first.ID)
	if !ok || child.ResumedFrom != "strength-7" {
		t.Fatalf("child after resume = %#v", child)
	}
}
func TestConcurrentResumeNeverCreatesDuplicateChild(t *testing.T) {
	coord := newResumeTestCoordinator(t)
	defer coord.Close()

	const callers = 16
	var wg sync.WaitGroup
	results := make(chan Handle, callers)
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := coord.Resume(context.Background(), "strength-7", "turn-new")
			if err != nil {
				errs <- err
				return
			}
			results <- h
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	var childID string
	for h := range results {
		if childID == "" {
			childID = h.ID
		}
		if h.ID != childID {
			t.Fatalf("duplicate resume children: %q and %q", childID, h.ID)
		}
	}
	if childID == "" {
		t.Fatal("no resume caller succeeded")
	}
	for err := range errs {
		if !errors.Is(err, ErrNotResumable) {
			t.Fatalf("concurrent resume error = %v", err)
		}
	}
	if got := len(coord.List()); got != 2 {
		t.Fatalf("retained agents = %d, want source + one child", got)
	}
}

func TestResumeAdmissionFailureRollsBackToInterrupted(t *testing.T) {
	coord := newResumeTestCoordinator(t)
	defer coord.Close()
	coord.SetEnabled(false)
	if _, err := coord.Resume(context.Background(), "strength-7", "turn-new"); !errors.Is(err, ErrSubagentsDisabled) {
		t.Fatalf("Resume() error = %v, want ErrSubagentsDisabled", err)
	}
	status, ok := coord.Get("strength-7")
	if !ok || status.State != StateInterrupted {
		t.Fatalf("source after failed resume = %#v", status)
	}
}
