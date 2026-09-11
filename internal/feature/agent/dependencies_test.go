package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

func TestDependencyWaitDoesNotConsumeConcurrencySlot(t *testing.T) {
	upstreamRelease := make(chan struct{})
	upstreamStarted := make(chan struct{})
	unrelatedStarted := make(chan struct{})
	coord := NewCoordinator(nil, nil, nil, nil,
		WithMaxConcurrency(2),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, messages []model.Message, _ turn.Sink) (turn.Result, error) {
				prompt := messages[len(messages)-1].Content
				switch {
				case strings.Contains(prompt, "Task: upstream"):
					close(upstreamStarted)
					select {
					case <-upstreamRelease:
					case <-ctx.Done():
						return turn.Result{}, ctx.Err()
					}
				case strings.Contains(prompt, "Task: unrelated"):
					close(unrelatedStarted)
				}
				return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
			}}, nil
		}),
	)
	defer coord.Close()

	ref := TurnRef{SessionID: "s", TurnID: "turn"}
	upstream, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "upstream"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-upstreamStarted:
	case <-time.After(time.Second):
		t.Fatal("upstream did not start")
	}

	downstream, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "downstream", DependsOn: []string{upstream.ID}})
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "unrelated"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-unrelatedStarted:
	case <-time.After(time.Second):
		t.Fatal("dependent child consumed concurrency before dependency completed")
	}

	close(upstreamRelease)
	for _, id := range []string{upstream.ID, downstream.ID, unrelated.ID} {
		wr, err := coord.Wait(context.Background(), id, time.Second)
		if err != nil || wr.Result == nil || wr.Result.Err != nil {
			t.Fatalf("wait %s = %+v err=%v", id, wr, err)
		}
	}
}

func TestDependencyWaitDoesNotConsumeQueueTimeout(t *testing.T) {
	release := make(chan struct{})
	coord := NewCoordinator(nil, nil, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(ctx context.Context, messages []model.Message, _ turn.Sink) (turn.Result, error) {
				if strings.Contains(messages[len(messages)-1].Content, "Task: upstream") {
					select {
					case <-release:
					case <-ctx.Done():
						return turn.Result{}, ctx.Err()
					}
				}
				return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
			}}, nil
		}),
	)
	defer coord.Close()

	ref := TurnRef{SessionID: "s", TurnID: "turn"}
	upstream, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "upstream"})
	if err != nil {
		t.Fatal(err)
	}
	downstream, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "downstream", DependsOn: []string{upstream.ID}, QueueTimeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	close(release)
	wr, err := coord.Wait(context.Background(), downstream.ID, time.Second)
	if err != nil || wr.Result == nil || wr.Result.Err != nil {
		t.Fatalf("downstream wait=%+v err=%v", wr, err)
	}
	if wr.Result.QueueDuration >= 50*time.Millisecond {
		t.Fatalf("queue duration=%s includes dependency wait", wr.Result.QueueDuration)
	}
	if wr.Result.TotalDuration < 50*time.Millisecond {
		t.Fatalf("total duration=%s omitted dependency wait", wr.Result.TotalDuration)
	}
}

func TestDependencyFailurePreventsDownstreamExecution(t *testing.T) {
	var downstreamRuns atomic.Int32
	coord := NewCoordinator(nil, nil, nil, nil,
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(_ context.Context, messages []model.Message, _ turn.Sink) (turn.Result, error) {
				prompt := messages[len(messages)-1].Content
				if strings.Contains(prompt, "Task: upstream") {
					return turn.Result{}, errors.New("upstream failed")
				}
				if strings.Contains(prompt, "Task: downstream") {
					downstreamRuns.Add(1)
				}
				return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "done"}}, nil
			}}, nil
		}),
	)
	defer coord.Close()
	ref := TurnRef{SessionID: "s", TurnID: "turn"}
	upstream, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "upstream"})
	if err != nil {
		t.Fatal(err)
	}
	downstream, err := coord.Spawn(context.Background(), Request{SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "downstream", DependsOn: []string{upstream.ID}})
	if err != nil {
		t.Fatal(err)
	}
	wr, err := coord.Wait(context.Background(), downstream.ID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if wr.Result == nil || !errors.Is(wr.Result.Err, ErrDependencyFailed) {
		t.Fatalf("downstream result=%+v", wr.Result)
	}
	if downstreamRuns.Load() != 0 {
		t.Fatalf("downstream runner invoked %d times", downstreamRuns.Load())
	}
}

func TestDependencyAdmissionRejectsMissingAndCrossTurnRefs(t *testing.T) {
	coord := NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()
	base := Request{SessionID: "s", ParentID: "turn-a", Profile: ProfileAgility, Task: "base"}
	upstream, err := coord.Spawn(context.Background(), base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coord.Spawn(context.Background(), Request{SessionID: "s", ParentID: "turn-a", Profile: ProfileAgility, Task: "missing", DependsOn: []string{"agility-999"}}); !errors.Is(err, ErrDependencyNotFound) {
		t.Fatalf("missing dependency error=%v", err)
	}
	if _, err := coord.Spawn(context.Background(), Request{SessionID: "s", ParentID: "turn-b", Profile: ProfileAgility, Task: "cross", DependsOn: []string{upstream.ID}}); !errors.Is(err, ErrDependencyScope) {
		t.Fatalf("cross-turn dependency error=%v", err)
	}
}

func TestNormalizeDependencyIDsPreservesFirstOccurrence(t *testing.T) {
	got := normalizeDependencyIDs([]string{" a ", "b", "a", "", " b "})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("normalized=%v", got)
	}
}
