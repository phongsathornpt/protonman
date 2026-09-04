package agent

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
)

type mockRunner struct {
	runFunc func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error)
}

func (m *mockRunner) Run(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, messages, sink)
	}
	return turn.Result{
		Message: model.Message{Role: model.RoleAssistant, Content: "mock output"},
		Rounds:  1,
	}, nil
}

type emptyRegistry struct{}

func (emptyRegistry) Lookup(_ string) (tool.Handler, bool) { return nil, false }
func (emptyRegistry) Definitions() []tool.Definition       { return nil }

func TestCoordinator_RunsSubagentInGoroutine(t *testing.T) {
	coord := NewCoordinator(
		nil,
		emptyRegistry{},
		nil,
		nil,
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					return turn.Result{
						Message: model.Message{Role: model.RoleAssistant, Content: "found 3 test files"},
						Rounds:  2,
					}, nil
				},
			}, nil
		}),
	)
	defer coord.Close()

	res, err := coord.Run(context.Background(), Request{
		Profile: ProfileExplorer,
		Task:    "find test files",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Summary != "found 3 test files" {
		t.Errorf("Summary = %q, want 'found 3 test files'", res.Summary)
	}
	if res.Rounds != 2 {
		t.Errorf("Rounds = %d, want 2", res.Rounds)
	}
	if res.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", res.Duration)
	}
}

func TestCoordinator_EnforcesConcurrencySemaphore(t *testing.T) {
	maxConcurrency := 2
	var currentActive atomic.Int32
	var peakActive atomic.Int32

	coord := NewCoordinator(
		nil,
		emptyRegistry{},
		nil,
		nil,
		WithMaxConcurrency(maxConcurrency),
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					cur := currentActive.Add(1)
					defer currentActive.Add(-1)

					// Update peak
					for {
						peak := peakActive.Load()
						if cur <= peak || peakActive.CompareAndSwap(peak, cur) {
							break
						}
					}

					time.Sleep(30 * time.Millisecond)
					return turn.Result{
						Message: model.Message{Role: model.RoleAssistant, Content: "done"},
					}, nil
				},
			}, nil
		}),
	)
	defer coord.Close()

	var wg sync.WaitGroup
	totalTasks := 6
	wg.Add(totalTasks)

	for i := 0; i < totalTasks; i++ {
		go func(taskNum int) {
			defer wg.Done()
			_, err := coord.Run(context.Background(), Request{
				Profile: ProfileExplorer,
				Task:    "concurrent task",
			})
			if err != nil {
				t.Errorf("task %d failed: %v", taskNum, err)
			}
		}(i)
	}

	wg.Wait()

	if peak := peakActive.Load(); peak > int32(maxConcurrency) {
		t.Fatalf("peak active subagents = %d, exceeded maxConcurrency %d", peak, maxConcurrency)
	}
}

func TestCoordinator_CancelsChildWhenParentContextCancels(t *testing.T) {
	childStarted := make(chan struct{})
	coord := NewCoordinator(
		nil,
		emptyRegistry{},
		nil,
		nil,
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					close(childStarted)
					<-ctx.Done()
					return turn.Result{}, ctx.Err()
				},
			}, nil
		}),
	)
	defer coord.Close()

	parentCtx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := coord.Run(parentCtx, Request{
			Profile: ProfileExplorer,
			Task:    "long task",
		})
		errCh <- err
	}()

	<-childStarted
	cancel() // cancel parent

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestCoordinator_EnforcesMaxDepth(t *testing.T) {
	coord := NewCoordinator(
		nil,
		emptyRegistry{},
		nil,
		nil,
		WithMaxDepth(1),
	)
	defer coord.Close()

	// Depth 1 is allowed
	_, err := coord.Run(context.Background(), Request{
		Profile: ProfileExplorer,
		Task:    "allowed depth",
		Depth:   1,
	})
	// Will fail because no runner factory / client, but NOT on depth validation
	if err != nil && err.Error() == "delegation depth 1 exceeds maximum depth 1" {
		t.Fatalf("unexpected depth error: %v", err)
	}

	// Depth 2 exceeds max depth 1
	_, err = coord.Run(context.Background(), Request{
		Profile: ProfileExplorer,
		Task:    "excessive depth",
		Depth:   2,
	})
	if err == nil || err.Error() != "delegation depth 2 exceeds maximum depth 1" {
		t.Fatalf("expected depth exceed error, got: %v", err)
	}
}

func TestCoordinator_SerializesMutatingWorkers(t *testing.T) {
	var currentWorkers atomic.Int32
	var peakWorkers atomic.Int32

	coord := NewCoordinator(
		nil,
		emptyRegistry{},
		nil,
		nil,
		WithMaxConcurrency(4), // High concurrency allowed overall
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					cur := currentWorkers.Add(1)
					defer currentWorkers.Add(-1)

					for {
						peak := peakWorkers.Load()
						if cur <= peak || peakWorkers.CompareAndSwap(peak, cur) {
							break
						}
					}

					time.Sleep(30 * time.Millisecond)
					return turn.Result{
						Message: model.Message{Role: model.RoleAssistant, Content: "worker done"},
					}, nil
				},
			}, nil
		}),
	)
	defer coord.Close()

	var wg sync.WaitGroup
	totalWorkers := 4
	wg.Add(totalWorkers)

	for i := 0; i < totalWorkers; i++ {
		go func(wNum int) {
			defer wg.Done()
			_, err := coord.Run(context.Background(), Request{
				Profile: ProfileWorker,
				Task:    "mutating task",
			})
			if err != nil {
				t.Errorf("worker %d error: %v", wNum, err)
			}
		}(i)
	}

	wg.Wait()

	// Mutating workers MUST be strictly serialized (peak == 1)
	if peak := peakWorkers.Load(); peak > 1 {
		t.Fatalf("mutating workers ran concurrently! peak = %d, want <= 1", peak)
	}
}

func TestCoordinator_ZeroGoroutineLeaks(t *testing.T) {
	coord := NewCoordinator(
		nil,
		emptyRegistry{},
		nil,
		nil,
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					select {
					case <-time.After(100 * time.Millisecond):
						return turn.Result{
							Message: model.Message{Role: model.RoleAssistant, Content: "done"},
						}, nil
					case <-ctx.Done():
						return turn.Result{}, ctx.Err()
					}
				},
			}, nil
		}),
	)

	// Baseline goroutine count
	runtime.GC()
	initialGoroutines := runtime.NumGoroutine()

	// Run 10 subagents with very short timeouts causing cancellations
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			_, _ = coord.Run(ctx, Request{
				Profile: ProfileExplorer,
				Task:    "short task",
			})
		}()
	}
	wg.Wait()

	// Close coordinator
	if err := coord.Close(); err != nil {
		t.Fatal(err)
	}

	// Wait for goroutines to drain
	deadline := time.Now().Add(1 * time.Second)
	var finalGoroutines int
	for time.Now().Before(deadline) {
		runtime.GC()
		finalGoroutines = runtime.NumGoroutine()
		if finalGoroutines <= initialGoroutines+1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("goroutine leak detected: initial = %d, final = %d", initialGoroutines, finalGoroutines)
	}
}

func TestCoordinator_EmitsLifecycleEvents(t *testing.T) {
	var events []Event
	var mu sync.Mutex

	coord := NewCoordinator(
		nil,
		emptyRegistry{},
		nil,
		nil,
		WithEventSink(func(ctx context.Context, ev Event) error {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
			return nil
		}),
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					return turn.Result{
						Message: model.Message{Role: model.RoleAssistant, Content: "completed task"},
					}, nil
				},
			}, nil
		}),
	)
	defer coord.Close()

	_, err := coord.Run(context.Background(), Request{
		Profile: ProfileReviewer,
		Task:    "review auth logic",
	})
	if err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	if events[0].Kind != EventAgentStarted {
		t.Errorf("first event = %v, want EventAgentStarted", events[0].Kind)
	}
	if events[len(events)-1].Kind != EventAgentCompleted {
		t.Errorf("last event = %v, want EventAgentCompleted", events[len(events)-1].Kind)
	}
}
