package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

type hardeningRunner struct {
	release <-chan struct{}
}

func (r hardeningRunner) Run(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
	if r.release != nil {
		select {
		case <-r.release:
		case <-ctx.Done():
			return turn.Result{}, ctx.Err()
		}
	}
	return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
}
func TestCoordinatorStressParentMailboxIsolation(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxConcurrency(8), WithMaxLiveAgents(64),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return hardeningRunner{}, nil
		}),
	)
	defer coord.Close()

	events, unsubscribe := coord.Subscribe(1)
	defer unsubscribe()
	for i := 0; i < 64; i++ {
		parent := fmt.Sprintf("turn-%d", i%8)
		if _, err := coord.Spawn(context.Background(), Request{
			ParentID: parent, Profile: ProfileAgility, Task: fmt.Sprintf("task-%d", i),
		}); err != nil {
			t.Fatalf("spawn %d: %v", i, err)
		}
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && len(coord.Active()) != 0 {
		time.Sleep(time.Millisecond)
	}
	if active := coord.Active(); len(active) != 0 {
		t.Fatalf("active agents after drain = %d", len(active))
	}
	for parent := 0; parent < 8; parent++ {
		parentID := fmt.Sprintf("turn-%d", parent)
		result, err := coord.WaitActivityForTurn(context.Background(), TurnRef{TurnID: parentID}, time.Second)
		if err != nil {
			t.Fatalf("wait %s: %v", parentID, err)
		}
		if result.Event == nil || result.Event.ParentID != parentID {
			t.Fatalf("wait %s woke with %#v", parentID, result.Event)
		}
		for _, status := range result.Agents {
			if status.ParentID != parentID {
				t.Fatalf("wait %s leaked status from %s", parentID, status.ParentID)
			}
		}
	}
	if got := len(coord.List()); got != 64 {
		t.Fatalf("retained agents = %d, want 64", got)
	}
	select {
	case event := <-events:
		if !terminalLifecycleEvent(event.Kind) {
			t.Fatalf("bounded subscriber retained non-terminal event %s", event.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("bounded subscriber lost terminal wakeup")
	}
}
func TestCoordinatorStressLiveLimitBackpressure(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxConcurrency(4), WithMaxLiveAgents(32), WithDefaultQueueTimeout(5*time.Second),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return hardeningRunner{release: release}, nil
		}),
	)
	defer coord.Close()

	handles := make([]Handle, 0, 32)
	for i := 0; i < 32; i++ {
		handle, err := coord.Spawn(context.Background(), Request{
			ParentID: "stress", Profile: ProfileAgility, Task: fmt.Sprintf("blocked-%d", i),
		})
		if err != nil {
			t.Fatalf("spawn %d: %v", i, err)
		}
		handles = append(handles, handle)
	}
	if _, err := coord.Spawn(context.Background(), Request{ParentID: "stress", Profile: ProfileAgility, Task: "overflow"}); !errors.Is(err, ErrLiveLimit) {
		t.Fatalf("overflow spawn error = %v, want ErrLiveLimit", err)
	}
	close(release)
	for _, handle := range handles {
		result, err := coord.Wait(context.Background(), handle.ID, 3*time.Second)
		if err != nil {
			t.Fatalf("wait %s: %v", handle.ID, err)
		}
		if result.TimedOut || result.State != StateCompleted {
			t.Fatalf("wait %s = %+v", handle.ID, result)
		}
	}
}

func TestCoordinatorEmitsRedactedLifecycleMetrics(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[MetricKind]int)
	observer := func(_ context.Context, event MetricEvent) {
		mu.Lock()
		seen[event.Kind]++
		mu.Unlock()
	}
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithDefaultWaitTimeout(5*time.Millisecond),
		WithMetricObserver(observer),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return hardeningRunner{}, nil
		}),
	)
	defer coord.Close()
	if _, err := coord.WaitActivityForTurn(context.Background(), TurnRef{TurnID: "empty-parent"}, 5*time.Millisecond); err != nil {
		t.Fatalf("wait timeout: %v", err)
	}
	handle, err := coord.Spawn(context.Background(), Request{ParentID: "metrics", Profile: ProfileAgility, Task: "finish"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coord.Wait(context.Background(), handle.ID, time.Second); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if seen[MetricWaitTimeout] != 1 {
		t.Fatalf("wait timeout metrics = %#v", seen)
	}
	if seen[MetricCompleted] != 1 {
		t.Fatalf("completion metrics = %#v", seen)
	}
}
