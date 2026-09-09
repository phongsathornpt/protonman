package agent

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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

func waitForTest(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for asynchronous condition")
		}
		time.Sleep(time.Millisecond)
	}
}

type emptyRegistry struct{}

func (emptyRegistry) Lookup(_ string) (tool.Handler, bool) { return nil, false }
func (emptyRegistry) Definitions() []tool.Definition       { return nil }

func TestCoordinatorMaxToolCallsOption(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithMaxToolCalls(7))
	if got, want := coord.maxToolCalls, 7; got != want {
		t.Fatalf("max tool calls = %d, want %d", got, want)
	}

	defaultCoord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	if got, want := defaultCoord.maxToolCalls, defaultMaxToolCalls; got != want {
		t.Fatalf("default max tool calls = %d, want %d", got, want)
	}
	if err := defaultCoord.Close(); err != nil {
		t.Fatalf("default coordinator Close() error = %v", err)
	}
	if err := coord.Close(); err != nil {
		t.Fatalf("configured coordinator Close() error = %v", err)
	}
}

func TestCoordinatorSubagentCapabilityToggle(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithEnabled(false))
	defer coord.Close()
	if coord.Enabled() {
		t.Fatal("coordinator should start disabled")
	}
	_, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "inspect router"})
	if !errors.Is(err, ErrSubagentsDisabled) {
		t.Fatalf("Spawn() error = %v, want ErrSubagentsDisabled", err)
	}
	coord.SetEnabled(true)
	if !coord.Enabled() {
		t.Fatal("coordinator should be enabled after SetEnabled(true)")
	}
	coord.SetEnabled(false)
	if coord.Enabled() {
		t.Fatal("coordinator should be disabled after SetEnabled(false)")
	}
}

func TestCoordinatorRunsSubagentInGoroutine(t *testing.T) {
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
		Profile: ProfileAgility,
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

func TestCoordinatorEnforcesConcurrencySemaphore(t *testing.T) {
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
				Profile: ProfileAgility,
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

func TestCoordinatorCancelsChildWhenParentContextCancels(t *testing.T) {
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
			Profile: ProfileAgility,
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

func TestCoordinatorSerializesMutatingWorkers(t *testing.T) {
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
				Profile: ProfileStrength,
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

func TestCoordinatorZeroGoroutineLeaks(t *testing.T) {
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
				Profile: ProfileAgility,
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

func TestCoordinatorCloseClosesSubscribers(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	events, unsubscribe := coord.Subscribe(1)

	if err := coord.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("subscriber channel remained open after coordinator close")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber channel was not closed")
	}
	unsubscribe() // must remain safe after coordinator-driven closure
}

func TestCoordinatorSubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	if err := coord.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	events, unsubscribe := coord.Subscribe(1)
	defer unsubscribe()
	if _, ok := <-events; ok {
		t.Fatal("Subscribe() after close returned an open channel")
	}
}

func TestCoordinatorTerminalEventsReplaceDroppedSubscriberWakeups(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	events, unsubscribe := coord.Subscribe(1)
	defer unsubscribe()

	coord.broadcast(Event{Kind: EventAgentProgress, AgentID: "a-1"})
	coord.broadcast(Event{Kind: EventAgentCompleted, AgentID: "a-1"})

	select {
	case ev := <-events:
		if ev.Kind != EventAgentCompleted {
			t.Fatalf("subscriber event = %s, want terminal completion", ev.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal subscriber event was dropped")
	}
}

func TestCoordinatorTerminalEventsReplaceDroppedSinkWakeups(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	coord.eventSink = func(context.Context, Event) error { return nil }
	coord.eventQueue = make(chan Event, 1)

	coord.emit(context.Background(), Event{Kind: EventAgentProgress, AgentID: "a-1"})
	coord.emit(context.Background(), Event{Kind: EventAgentFailed, AgentID: "a-1"})

	select {
	case ev := <-coord.eventQueue:
		if ev.Kind != EventAgentFailed {
			t.Fatalf("sink event = %s, want terminal failure", ev.Kind)
		}
	default:
		t.Fatal("terminal sink event was dropped")
	}
}

func TestCoordinatorEmitsLifecycleEvents(t *testing.T) {
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
		Profile: ProfileAgility,
		Task:    "review auth logic",
	})
	if err != nil {
		t.Fatal(err)
	}

	waitForTest(t, 250*time.Millisecond, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) >= 3
	})
	mu.Lock()
	defer mu.Unlock()
	if events[0].Kind != EventAgentQueued {
		t.Errorf("first event = %v, want EventAgentQueued", events[0].Kind)
	}
	if events[1].Kind != EventAgentStarted {
		t.Errorf("second event = %v, want EventAgentStarted", events[1].Kind)
	}
	if events[len(events)-1].Kind != EventAgentCompleted {
		t.Errorf("last event = %v, want EventAgentCompleted", events[len(events)-1].Kind)
	}
}

func TestCoordinatorConcurrentCloseAndRun(t *testing.T) {
	coord := NewCoordinator(
		nil,
		emptyRegistry{},
		nil,
		nil,
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					select {
					case <-ctx.Done():
						return turn.Result{}, ctx.Err()
					case <-time.After(10 * time.Millisecond):
						return turn.Result{
							Message: model.Message{Role: model.RoleAssistant, Content: "done"},
						}, nil
					}
				},
			}, nil
		}),
	)

	var wg sync.WaitGroup
	numCallers := 50
	wg.Add(numCallers)

	for i := 0; i < numCallers; i++ {
		go func() {
			defer wg.Done()
			_, _ = coord.Run(context.Background(), Request{
				Profile: ProfileAgility,
				Task:    "race task",
			})
		}()
	}

	// Concurrently close the coordinator while callers are in flight
	time.Sleep(2 * time.Millisecond)
	_ = coord.Close()

	wg.Wait()
}

func TestCoordinatorResilientEmitOnCancel(t *testing.T) {
	var failedEmitted atomic.Bool
	coord := NewCoordinator(
		nil,
		emptyRegistry{},
		nil,
		nil,
		WithEventSink(func(ctx context.Context, ev Event) error {
			if ev.Kind == EventAgentFailed {
				// Even if childCtx was canceled, ctx passed to emit must remain usable
				if ctx.Err() == nil {
					failedEmitted.Store(true)
				}
			}
			return nil
		}),
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					<-ctx.Done()
					return turn.Result{}, ctx.Err()
				},
			}, nil
		}),
	)
	defer coord.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	_, _ = coord.Run(ctx, Request{
		Profile: ProfileAgility,
		Task:    "cancel task",
	})

	time.Sleep(20 * time.Millisecond)
	if !failedEmitted.Load() {
		t.Errorf("expected resilient emit of EventAgentFailed with valid context on cancellation")
	}
}

func TestCoordinatorReadOnlyProfileInheritsAskModeForNetworkTools(t *testing.T) {
	baseReg := staticRegistry{
		handlers: map[string]tool.Handler{
			"web": dummyHandler{def: tool.Definition{Name: "web", Kind: tool.KindWeb, Description: "web"}},
		},
	}

	promptCalls := 0
	var callErr error
	coord := NewCoordinator(
		nil,
		baseReg,
		nil,
		nil,
		WithPermissionMode(permission.ModeAsk),
		WithPermissionPrompt(func(context.Context, permission.Request) (permission.Resolution, error) {
			promptCalls++
			return permission.Resolution{Action: permission.ActionDeny, Reason: "network not approved"}, nil
		}),
		WithRunnerFactory(func(_ Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				call, _ := tool.NewCall("fetch-1", "web", []byte(`{"action":"fetch","url":"https://example.com"}`))
				_, callErr = tools.Call(ctx, call)
				return turn.Result{Message: model.Message{Content: "done"}}, nil
			}}, nil
		}),
	)
	defer coord.Close()

	if _, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "inspect web"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if promptCalls != 1 {
		t.Fatalf("permission prompt calls = %d, want 1", promptCalls)
	}
	if !errors.Is(callErr, toolcall.ErrPermissionDenied) {
		t.Fatalf("web error = %v, want permission denied", callErr)
	}
}

func TestCoordinatorWorkerInheritsAlwaysApproveMode(t *testing.T) {
	baseReg := staticRegistry{
		handlers: map[string]tool.Handler{
			"edit": dummyHandler{def: tool.Definition{Name: "edit", Kind: tool.KindEdit}},
		},
	}

	var callErr error
	coord := NewCoordinator(
		nil,
		baseReg,
		nil,
		nil,
		WithPermissionMode(permission.ModeAlwaysApprove),
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					call, _ := tool.NewCall("c-1", "edit", []byte(`{"action":"write"}`))
					_, callErr = tools.Call(ctx, call)
					return turn.Result{Message: model.Message{Content: "wrote file"}}, nil
				},
			}, nil
		}),
	)
	defer coord.Close()

	res, err := coord.Run(context.Background(), Request{
		Profile: ProfileStrength,
		Task:    "write file task",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if callErr != nil {
		t.Fatalf("expected tool call to succeed in ModeAlwaysApprove, got error: %v", callErr)
	}
	if res.Summary != "wrote file" {
		t.Errorf("summary = %q, want 'wrote file'", res.Summary)
	}
}

func TestCoordinatorSubagentInheritsCallGuard(t *testing.T) {
	baseReg := staticRegistry{
		handlers: map[string]tool.Handler{
			"edit": dummyHandler{def: tool.Definition{Name: "edit", Kind: tool.KindEdit}},
		},
	}

	readOnlyGuard := func(_ context.Context, req permission.Request) error {
		if req.ToolKind != permission.ToolRead {
			return errors.New("guard: plan mode is read-only")
		}
		return nil
	}

	var callErr error
	coord := NewCoordinator(
		nil,
		baseReg,
		nil,
		nil,
		WithPermissionMode(permission.ModeAlwaysApprove),
		WithCallGuard(readOnlyGuard),
		WithRunnerFactory(func(p Profile, tools *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{
				runFunc: func(ctx context.Context, messages []model.Message, sink turn.Sink) (turn.Result, error) {
					call, _ := tool.NewCall("c-1", "edit", []byte(`{"action":"write"}`))
					_, callErr = tools.Call(ctx, call)
					return turn.Result{Message: model.Message{Content: "done"}}, nil
				},
			}, nil
		}),
	)
	defer coord.Close()

	_, err := coord.Run(context.Background(), Request{
		Profile: ProfileStrength,
		Task:    "write file task",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if callErr == nil {
		t.Fatal("expected tool call to be blocked by CallGuard, but it succeeded")
	}
	if !strings.Contains(callErr.Error(), "guard: plan mode is read-only") {
		t.Errorf("callErr = %v, want 'guard: plan mode is read-only'", callErr)
	}
}

func TestCoordinatorWorkspaceGateCancelsExclusiveWait(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithMaxConcurrency(2))
	releaseReader, err := coord.acquireWorkspace(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseReader()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := coord.acquireWorkspace(ctx, true)
		done <- err
	}()
	time.Sleep(5 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("workspace acquire error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("exclusive workspace wait ignored cancellation")
	}
}

func TestCoordinatorWorkspaceWriterBlocksNewReaders(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithMaxConcurrency(2))
	defer coord.Close()

	releaseReader, err := coord.acquireWorkspace(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}

	writerAcquired := make(chan func(), 1)
	go func() {
		release, err := coord.acquireWorkspace(context.Background(), true)
		if err == nil {
			writerAcquired <- release
		}
	}()
	waitForTest(t, time.Second, func() bool { return len(coord.wsWriter) == 1 && len(coord.wsAdmission) == 1 })

	readerAcquired := make(chan func(), 1)
	go func() {
		release, err := coord.acquireWorkspace(context.Background(), false)
		if err == nil {
			readerAcquired <- release
		}
	}()
	select {
	case release := <-readerAcquired:
		release()
		t.Fatal("reader bypassed queued writer")
	case <-time.After(20 * time.Millisecond):
	}

	releaseReader()
	var releaseWriter func()
	select {
	case releaseWriter = <-writerAcquired:
	case <-time.After(time.Second):
		t.Fatal("writer did not acquire workspace after readers drained")
	}
	select {
	case release := <-readerAcquired:
		release()
		t.Fatal("reader acquired workspace while writer was active")
	case <-time.After(20 * time.Millisecond):
	}
	releaseWriter()
	select {
	case release := <-readerAcquired:
		release()
	case <-time.After(time.Second):
		t.Fatal("reader did not resume after writer released workspace")
	}
}

func TestCoordinatorWorkspaceGateAllowsConcurrentReaders(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithMaxConcurrency(2))
	releaseA, err := coord.acquireWorkspace(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseA()
	releaseB, err := coord.acquireWorkspace(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	releaseB()
}

func TestCoordinatorExecutionTimeoutReturnsForNonCooperativeRunner(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(20*time.Millisecond),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
				<-release
				return turn.Result{}, nil
			}}, nil
		}),
	)

	started := time.Now()
	_, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "ignore cancellation"})
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want deadline exceeded", err)
	}
	if elapsed > 150*time.Millisecond {
		t.Fatalf("Run() elapsed = %v, hard timeout did not return promptly", elapsed)
	}
	close(release)
	if err := coord.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestCoordinatorQueueWaitDoesNotConsumeExecutionTimeout(t *testing.T) {
	firstRelease := make(chan struct{})
	secondStarted := make(chan time.Time, 1)
	var calls atomic.Int32
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxConcurrency(1),
		WithDefaultQueueTimeout(time.Second),
		WithMaxRuntime(500*time.Millisecond),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				if calls.Add(1) == 1 {
					<-firstRelease
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "first"}}, nil
				}
				secondStarted <- time.Now()
				select {
				case <-time.After(50 * time.Millisecond):
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "second"}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer coord.Close()

	firstDone := make(chan error, 1)
	go func() {
		_, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "first", Timeout: 500 * time.Millisecond})
		firstDone <- err
	}()
	time.Sleep(20 * time.Millisecond)

	secondDone := make(chan error, 1)
	queuedAt := time.Now()
	go func() {
		_, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "second", Timeout: 80 * time.Millisecond})
		secondDone <- err
	}()
	time.Sleep(60 * time.Millisecond)
	close(firstRelease)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	startAt := <-secondStarted
	if wait := startAt.Sub(queuedAt); wait < 50*time.Millisecond {
		t.Fatalf("second queue wait = %v, want meaningful queue delay", wait)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second Run() error = %v; queue wait consumed execution budget", err)
	}
}

func TestCoordinatorParentDeadlineStillBoundsExecution(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(time.Second),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				<-ctx.Done()
				return turn.Result{}, ctx.Err()
			}}, nil
		}),
	)
	defer coord.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := coord.Run(ctx, Request{Profile: ProfileAgility, Task: "parent deadline"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("Run() elapsed = %v, parent deadline did not bound execution", elapsed)
	}
}

func TestRequestRejectsNegativeTimeouts(t *testing.T) {
	for _, req := range []Request{
		{Profile: ProfileAgility, Task: "bad execution timeout", Timeout: -time.Second},
		{Profile: ProfileAgility, Task: "bad queue timeout", QueueTimeout: -time.Second},
	} {
		if err := req.Validate(); err == nil {
			t.Fatalf("Validate(%+v) error = nil, want validation error", req)
		}
	}
}

func TestCoordinatorClampsRequestedTimeoutToConfiguredMaximum(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(20*time.Millisecond),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
				<-release
				return turn.Result{}, nil
			}}, nil
		}),
	)
	started := time.Now()
	_, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "clamp", Timeout: time.Second})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want configured maximum deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("Run() elapsed = %v, requested timeout escaped configured maximum", elapsed)
	}
	close(release)
	_ = coord.Close()
}

func TestCoordinatorBlockedEventSinkDoesNotHoldExecution(t *testing.T) {
	releaseSink := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(30*time.Millisecond),
		WithEventSink(func(context.Context, Event) error {
			<-releaseSink
			return nil
		}),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				<-ctx.Done()
				return turn.Result{}, ctx.Err()
			}}, nil
		}),
	)
	started := time.Now()
	_, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "blocked observer"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("Run() elapsed = %v, blocked event sink held execution", elapsed)
	}
	if err := coord.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	close(releaseSink)
}

func TestCoordinatorCloseIsBoundedForNonCooperativeRunner(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(20*time.Millisecond),
		WithCloseTimeout(30*time.Millisecond),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
				<-release
				return turn.Result{}, nil
			}}, nil
		}),
	)
	_, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "ignore close"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want deadline exceeded", err)
	}
	started := time.Now()
	closeErr := coord.Close()
	if !errors.Is(closeErr, ErrShutdownTimeout) {
		t.Fatalf("Close() error = %v, want ErrShutdownTimeout", closeErr)
	}
	var timeoutErr *ShutdownTimeoutError
	if !errors.As(closeErr, &timeoutErr) || timeoutErr.ActiveAgents != 1 {
		t.Fatalf("Close() error = %#v, want one active agent", closeErr)
	}
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("Close() elapsed = %v, want bounded shutdown", elapsed)
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for len(coord.Active()) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if active := coord.Active(); len(active) != 0 {
		t.Fatalf("active subagents after releasing runner = %d", len(active))
	}
}

func TestCoordinatorQueueTimeoutReportsLifecycleMetrics(t *testing.T) {
	release := make(chan struct{})
	events := make(chan Event, 8)
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxConcurrency(1),
		WithMaxRuntime(time.Second),
		WithDefaultQueueTimeout(20*time.Millisecond),
		WithEventSink(func(_ context.Context, ev Event) error { events <- ev; return nil }),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				select {
				case <-release:
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer coord.Close()
	firstDone := make(chan error, 1)
	go func() {
		_, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "holder", QueueTimeout: time.Second})
		firstDone <- err
	}()
	time.Sleep(10 * time.Millisecond)
	res, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "queued timeout"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued Run() error = %v, want deadline exceeded", err)
	}
	if res.QueueDuration <= 0 || res.TotalDuration < res.QueueDuration || res.Duration != 0 {
		t.Fatalf("queue timeout metrics = %+v", res)
	}
	var failure Event
	waitForTest(t, 250*time.Millisecond, func() bool {
		for {
			select {
			case ev := <-events:
				if ev.Kind == EventAgentFailed && ev.AgentID == res.AgentID {
					failure = ev
					return true
				}
			default:
				return false
			}
		}
	})
	if failure.QueueDuration <= 0 || failure.TotalDuration < failure.QueueDuration {
		t.Fatalf("failure event metrics = %+v", failure)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("holder Run() error = %v", err)
	}
}

func TestCoordinatorRepeatedCloseStillReportsActiveWorker(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(10*time.Millisecond),
		WithCloseTimeout(15*time.Millisecond),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
				<-release
				return turn.Result{}, nil
			}}, nil
		}),
	)
	_, _ = coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "repeat close"})
	if err := coord.Close(); err == nil {
		t.Fatal("first Close() error = nil, want active-worker timeout")
	}
	if err := coord.Close(); err == nil {
		t.Fatal("second Close() error = nil while worker is still active")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for len(coord.Active()) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := coord.Close(); err != nil {
		t.Fatalf("Close() after worker exit = %v", err)
	}
}

func TestCoordinatorWaitTimeoutDoesNotCancelSpawnedAgent(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(time.Second),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				select {
				case <-release:
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "background"})
	if err != nil {
		t.Fatal(err)
	}
	wr, err := coord.Wait(context.Background(), h.ID, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if wr.State != StateQueued && wr.State != StateRunning {
		t.Fatalf("wait state = %q", wr.State)
	}
	if _, ok := coord.Get(h.ID); !ok {
		t.Fatal("spawned agent disappeared after wait timeout")
	}
	close(release)
	wr, err = coord.Wait(context.Background(), h.ID, time.Second)
	if err != nil || wr.State != StateCompleted || wr.Result == nil {
		t.Fatalf("completion = %+v err=%v", wr, err)
	}
}

func TestCoordinatorMissingAgentUsesTypedError(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	if err := coord.Cancel("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cancel() error = %v, want ErrNotFound", err)
	}
	if _, err := coord.Wait(context.Background(), "missing", time.Millisecond); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Wait() error = %v, want ErrNotFound", err)
	}
}

func TestCoordinatorCancelExposesCancelingUntilRunnerStops(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
				<-release
				return turn.Result{}, context.Canceled
			}}, nil
		}),
	)
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "cancel state"})
	if err != nil {
		t.Fatal(err)
	}
	waitForTest(t, time.Second, func() bool {
		st, ok := coord.Get(h.ID)
		return ok && st.State == StateRunning
	})
	if err := coord.Cancel(h.ID); err != nil {
		t.Fatal(err)
	}
	st, ok := coord.Get(h.ID)
	if !ok || st.State != StateCanceling {
		t.Fatalf("status after Cancel() = %+v, ok=%v; want canceling", st, ok)
	}
	close(release)
	wr, err := coord.Wait(context.Background(), h.ID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if wr.State != StateCanceled {
		t.Fatalf("terminal state = %s, want canceled", wr.State)
	}
}

func TestCoordinatorCancelSpawnedAgent(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				<-ctx.Done()
				return turn.Result{}, ctx.Err()
			}}, nil
		}),
	)
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "cancel me"})
	if err != nil {
		t.Fatal(err)
	}
	if err := coord.Cancel(h.ID); err != nil {
		t.Fatal(err)
	}
	wr, err := coord.Wait(context.Background(), h.ID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if wr.State != StateCanceled || wr.Result == nil || !errors.Is(wr.Result.Err, context.Canceled) {
		t.Fatalf("canceled wait result = %+v", wr)
	}
}

func TestCoordinatorRetainsTerminalResultsOutsideActive(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return mockSubagentRunnerCompat("retained"), nil
		}),
	)
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "finish"})
	if err != nil {
		t.Fatal(err)
	}
	wr, err := coord.Wait(context.Background(), h.ID, time.Second)
	if err != nil || wr.State != StateCompleted {
		t.Fatalf("wait = %+v err=%v", wr, err)
	}
	if got := len(coord.Active()); got != 0 {
		t.Fatalf("Active() = %d, want 0", got)
	}
	found := false
	for _, st := range coord.List() {
		if st.ID == h.ID && st.State == StateCompleted {
			found = true
		}
	}
	if !found {
		t.Fatal("completed agent not retained in List()")
	}
}

type mockSubagentRunnerCompat string

func (m mockSubagentRunnerCompat) Run(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
	return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: string(m)}, Rounds: 1}, nil
}

func TestCoordinatorDefaultWaitTimeoutDoesNotCancel(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithDefaultWaitTimeout(20*time.Millisecond),
		WithMaxRuntime(time.Second),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				select {
				case <-release:
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "wait default"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	wr, err := coord.Wait(context.Background(), h.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 15*time.Millisecond {
		t.Fatalf("default wait returned too early")
	}
	if wr.State != StateQueued && wr.State != StateRunning {
		t.Fatalf("state=%q", wr.State)
	}
	close(release)
	_, _ = coord.Wait(context.Background(), h.ID, time.Second)
}

func TestCoordinatorMaxLiveAgents(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxLiveAgents(1),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				select {
				case <-release:
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "one"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "two"}); !errors.Is(err, ErrLiveLimit) {
		t.Fatalf("second Spawn error=%v, want ErrLiveLimit", err)
	}
	close(release)
	_, _ = coord.Wait(context.Background(), h.ID, time.Second)
	if _, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "three"}); err != nil {
		t.Fatalf("spawn after completion: %v", err)
	}
}

func TestCoordinatorCompletedResultTTL(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithResultTTL(20*time.Millisecond),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) { return mockSubagentRunnerCompat("ttl"), nil }),
	)
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "ttl"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coord.Wait(context.Background(), h.ID, time.Second); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := coord.Get(h.ID); ok {
		t.Fatal("expired terminal result still retained")
	}
}

func TestCoordinatorBoundsRetainedTerminalRecords(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRetainedAgents(2),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) { return &mockRunner{}, nil }),
	)
	defer coord.Close()
	for i := 0; i < 4; i++ {
		if _, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: fmt.Sprintf("task-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	got := coord.List()
	if len(got) != 2 {
		t.Fatalf("retained=%d, want 2: %#v", len(got), got)
	}
	for _, st := range got {
		if !st.State.Terminal() {
			t.Fatalf("unexpected live state: %#v", st)
		}
	}
}

func TestCoordinatorBoundsRetainedTerminalRecordsWhenTTLDisabled(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRetainedAgents(2),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) { return &mockRunner{}, nil }),
	)
	defer coord.Close()
	coord.resultTTL = 0
	for i := 0; i < 4; i++ {
		if _, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: fmt.Sprintf("no-ttl-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if got := coord.List(); len(got) != 2 {
		t.Fatalf("retained=%d, want 2 with TTL disabled: %#v", len(got), got)
	}
}

func TestTruncateSummaryPreservesUTF8AndByteLimit(t *testing.T) {
	input := strings.Repeat("ภาษาไทย🙂", 5000)
	got := truncateSummary(input, maxSummaryBytes)
	if !utf8.ValidString(got) {
		t.Fatal("truncateSummary() returned invalid UTF-8")
	}
	if len(got) > maxSummaryBytes {
		t.Fatalf("truncateSummary() bytes = %d, want <= %d", len(got), maxSummaryBytes)
	}
	if !strings.HasSuffix(got, "\n... [output truncated]") {
		t.Fatal("truncateSummary() missing truncation suffix")
	}
}

func TestCoordinatorTerminalStatusIncludesReason(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(20*time.Millisecond),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				<-ctx.Done()
				return turn.Result{}, ctx.Err()
			}}, nil
		}),
	)
	defer coord.Close()
	res, err := coord.Run(context.Background(), Request{Profile: ProfileAgility, Task: "slow review"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error=%v", err)
	}
	deadline := time.Now().Add(time.Second)
	var st AgentStatus
	var ok bool
	for time.Now().Before(deadline) {
		st, _, ok = coord.Lookup(res.AgentID)
		if ok && st.State.Terminal() {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !ok || !st.State.Terminal() || st.Reason == "" {
		t.Fatalf("terminal status=%+v", st)
	}
}

func TestTerminalReasonPreservesUTF8Boundary(t *testing.T) {
	err := errors.New(strings.Repeat("วิเคราะห์", 30))
	got := terminalReason(err)
	if !utf8.ValidString(got) {
		t.Fatalf("terminalReason returned invalid UTF-8: %q", got)
	}
	if len(got) > 80 {
		t.Fatalf("terminalReason bytes=%d, want <=80", len(got))
	}
}

func TestCoordinatorCancelByParentScopesCancellation(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, nil, nil, nil,
		WithMaxConcurrency(3),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				select {
				case <-release:
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer func() { close(release); _ = coord.Close() }()

	first, err := coord.Spawn(context.Background(), Request{ParentID: "turn-a", Profile: ProfileAgility, Task: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := coord.Spawn(context.Background(), Request{ParentID: "turn-b", Profile: ProfileAgility, Task: "second"})
	if err != nil {
		t.Fatal(err)
	}

	if got := coord.CancelByParent("turn-a"); got != 1 {
		t.Fatalf("CancelByParent()=%d, want 1", got)
	}
	firstStatus, _ := coord.Get(first.ID)
	if firstStatus.State != StateCanceling && firstStatus.State != StateCanceled {
		t.Fatalf("first state=%s", firstStatus.State)
	}
	secondStatus, _ := coord.Get(second.ID)
	if secondStatus.State == StateCanceling || secondStatus.State == StateCanceled {
		t.Fatalf("second state=%s, want unaffected", secondStatus.State)
	}
}

func TestParentIDContextRoundTrip(t *testing.T) {
	ctx := WithTurnRef(context.Background(), TurnRef{TurnID: " turn-42 "})
	if got := ParentIDFromContext(ctx); got != "turn-42" {
		t.Fatalf("ParentIDFromContext()=%q", got)
	}
}

func TestTerminalReasonClassifiesKnownFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "permission", err: fmt.Errorf("wrapped: %w", toolcall.ErrPermissionDenied), want: "permission denied"},
		{name: "model unavailable", err: sdk.NewProviderError("openai", 404, "model_not_found", "missing"), want: "model unavailable"},
		{name: "rate limit", err: sdk.NewProviderError("openai", 429, "rate_limit", "slow down"), want: "rate limited"},
		{name: "invalid request detail", err: sdk.NewProviderError("openai", 400, "invalid_request", "unsupported tool_choice: required"), want: "invalid model request: unsupported tool_choice: required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := terminalReason(tt.err); got != tt.want {
				t.Fatalf("terminalReason()=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetEnabledLinearizesWithSpawnAdmission(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()

	coord.agentsMu.Lock()
	done := make(chan struct{})
	go func() {
		coord.SetEnabled(false)
		close(done)
	}()
	select {
	case <-done:
		coord.agentsMu.Unlock()
		t.Fatal("SetEnabled returned while spawn admission lock was held")
	case <-time.After(20 * time.Millisecond):
	}
	coord.agentsMu.Unlock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SetEnabled did not complete after admission lock was released")
	}
	if coord.Enabled() {
		t.Fatal("coordinator remained enabled")
	}
}

func TestWaitActivityTimeoutIsNonFatal(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(time.Second),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				select {
				case <-release:
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer func() { close(release); _ = coord.Close() }()

	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "background"})
	if err != nil {
		t.Fatal(err)
	}
	waitForTest(t, time.Second, func() bool {
		status, ok := coord.Get(h.ID)
		return ok && status.State == StateRunning
	})

	wr, err := coord.WaitActivity(context.Background(), 20*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitActivity() error = %v", err)
	}
	if !wr.TimedOut || wr.Event != nil {
		t.Fatalf("WaitActivity() = %+v, want non-fatal timeout", wr)
	}
	status, ok := coord.Get(h.ID)
	if !ok || status.State != StateRunning {
		t.Fatalf("child state after wait timeout = %+v, ok=%v", status, ok)
	}
}

func TestWaitActivityConsumesCompletionRecordedBeforeWait(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
				return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "finished early"}}, nil
			}}, nil
		}),
	)
	defer coord.Close()

	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "finish early"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coord.Wait(context.Background(), h.ID, time.Second); err != nil {
		t.Fatal(err)
	}

	wr, err := coord.WaitActivity(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("WaitActivity() error = %v", err)
	}
	if wr.TimedOut || wr.Event == nil || wr.Event.AgentID != h.ID || wr.Event.Kind != EventAgentCompleted {
		t.Fatalf("WaitActivity() = %+v, want retained completion activity", wr)
	}
}

func TestWaitActivityCallerCancellationDoesNotCancelChild(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxRuntime(time.Second),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
				select {
				case <-release:
					return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
				case <-ctx.Done():
					return turn.Result{}, ctx.Err()
				}
			}}, nil
		}),
	)
	defer func() { close(release); _ = coord.Close() }()

	h, err := coord.Spawn(context.Background(), Request{Profile: ProfileAgility, Task: "survive caller cancel"})
	if err != nil {
		t.Fatal(err)
	}
	waitForTest(t, time.Second, func() bool {
		status, ok := coord.Get(h.ID)
		return ok && status.State == StateRunning
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coord.WaitActivity(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitActivity() error = %v, want context canceled", err)
	}
	status, ok := coord.Get(h.ID)
	if !ok || status.State != StateRunning {
		t.Fatalf("child state after caller cancellation = %+v, ok=%v", status, ok)
	}
}

func TestWaitActivityForParentIgnoresUnrelatedActivity(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithDefaultWaitTimeout(20*time.Millisecond))
	defer coord.Close()

	coord.recordActivity(Event{Kind: EventAgentCompleted, AgentID: "agent-a", ParentID: "turn-a"})
	wr, err := coord.WaitActivityForTurn(context.Background(), TurnRef{TurnID: "turn-b"}, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitActivityForParent() error = %v", err)
	}
	if !wr.TimedOut || wr.Event != nil {
		t.Fatalf("unrelated activity woke scoped wait: %+v", wr)
	}

	wr, err = coord.WaitActivityForTurn(context.Background(), TurnRef{TurnID: "turn-a"}, time.Second)
	if err != nil {
		t.Fatalf("WaitActivityForParent() error = %v", err)
	}
	if wr.TimedOut || wr.Event == nil || wr.Event.AgentID != "agent-a" {
		t.Fatalf("scoped activity = %+v, want agent-a", wr)
	}
}

func TestWaitActivityGlobalStillObservesScopedActivity(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()

	coord.recordActivity(Event{Kind: EventAgentFailed, AgentID: "agent-a", ParentID: "turn-a"})
	wr, err := coord.WaitActivity(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("WaitActivity() error = %v", err)
	}
	if wr.Event == nil || wr.Event.AgentID != "agent-a" || wr.Event.ParentID != "turn-a" {
		t.Fatalf("global activity = %+v", wr)
	}
}

func TestCancelTurnPolicyCancelsOwnedChildrenOnly(t *testing.T) {
	started := make(chan struct{})
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
		return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
			select {
			case <-started:
			default:
				close(started)
			}
			<-ctx.Done()
			return turn.Result{}, ctx.Err()
		}}, nil
	}))
	defer coord.Close()
	h, err := coord.Spawn(context.Background(), Request{SessionID: "session-a", ParentID: "turn-1", Profile: ProfileAgility, Task: "wait"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("child did not start")
	}
	if got := coord.CancelTurn(TurnRef{SessionID: "session-a", TurnID: "turn-1"}, CancelTurnOnly); got != 0 {
		t.Fatalf("CancelTurn(turn-only)=%d", got)
	}
	if got := coord.CancelTurn(TurnRef{SessionID: "session-a", TurnID: "turn-1"}, CancelTurnAndChildren); got != 1 {
		t.Fatalf("CancelTurn(with-children)=%d", got)
	}
	wr, err := coord.Wait(context.Background(), h.ID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if wr.State != StateCanceled {
		t.Fatalf("state=%s, want canceled", wr.State)
	}
}

func TestCancelSessionAndWaitIsSessionScoped(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
		return &mockRunner{runFunc: func(ctx context.Context, _ []model.Message, _ turn.Sink) (turn.Result, error) {
			<-ctx.Done()
			return turn.Result{}, ctx.Err()
		}}, nil
	}), WithMaxConcurrency(2))
	defer coord.Close()
	a, err := coord.Spawn(context.Background(), Request{SessionID: "session-a", ParentID: "turn-a", Profile: ProfileAgility, Task: "a"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := coord.Spawn(context.Background(), Request{SessionID: "session-b", ParentID: "turn-b", Profile: ProfileAgility, Task: "b"})
	if err != nil {
		t.Fatal(err)
	}
	waitForTest(t, time.Second, func() bool {
		sa, oka := coord.Get(a.ID)
		sb, okb := coord.Get(b.ID)
		return oka && okb && sa.State == StateRunning && sb.State == StateRunning
	})
	count, err := coord.CancelSessionAndWait(context.Background(), "session-a")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count=%d", count)
	}
	sa, _ := coord.Get(a.ID)
	sb, _ := coord.Get(b.ID)
	if sa.State != StateCanceled {
		t.Fatalf("session-a state=%s", sa.State)
	}
	if sb.State != StateRunning {
		t.Fatalf("session-b state=%s", sb.State)
	}
	_ = coord.Cancel(b.ID)
}

func TestSpawnSnapshotsChildToolRuntimePolicy(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithToolRuntimePolicy(17*time.Millisecond, 23*time.Millisecond, nil),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) { return &mockRunner{}, nil }),
	)
	defer coord.Close()
	coord.SetPermissionMode(permission.ModeAlwaysApprove)
	h, err := coord.Spawn(context.Background(), Request{SessionID: "session-a", ParentID: "turn-1", Profile: ProfileAgility, Task: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	coord.SetPermissionMode(permission.ModeAsk)
	coord.agentsMu.RLock()
	entry := coord.agents[h.ID]
	spec := entry.toolRuntime
	coord.agentsMu.RUnlock()
	if spec.permissionMode != permission.ModeAlwaysApprove {
		t.Fatalf("permission mode=%s, want always-approve admission snapshot", spec.permissionMode)
	}
	if spec.permissionTimeout != 17*time.Millisecond || spec.executionTimeout != 23*time.Millisecond {
		t.Fatalf("runtime timeouts=%s/%s", spec.permissionTimeout, spec.executionTimeout)
	}
}
