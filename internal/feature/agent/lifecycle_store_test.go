package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

type recordingLifecycleStore struct {
	mu     sync.Mutex
	events []LifecycleEvent
	err    error
}

func (s *recordingLifecycleStore) AppendLifecycleEvent(_ context.Context, event LifecycleEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.events = append(s.events, event)
	return nil
}

func (s *recordingLifecycleStore) LoadLifecycleEvents(context.Context, string) ([]LifecycleEvent, error) {
	return nil, nil
}
func TestSpawnRequiresDurableAdmissionWhenStoreConfigured(t *testing.T) {
	store := &recordingLifecycleStore{err: errors.New("disk unavailable")}
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithLifecycleEventStore(store),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return hardeningRunner{}, nil
		}),
	)
	defer coord.Close()
	_, err := coord.Spawn(context.Background(), Request{
		SessionID: "session-a", ParentID: "turn-1", Profile: ProfileAgility, Task: "inspect",
	})
	if err == nil {
		t.Fatal("expected durable admission failure")
	}
	if got := len(coord.List()); got != 0 {
		t.Fatalf("retained agents after failed admission = %d", got)
	}
}

func TestLifecycleStoreReceivesVersionedRunEvents(t *testing.T) {
	store := &recordingLifecycleStore{}
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithLifecycleEventStore(store),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return hardeningRunner{}, nil
		}),
	)
	defer coord.Close()
	handle, err := coord.Spawn(context.Background(), Request{
		SessionID: "session-a", ParentID: "turn-1", Profile: ProfileAgility, Task: "inspect",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := coord.Wait(context.Background(), handle.ID, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != StateCompleted {
		t.Fatalf("state = %s", result.State)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.events) != 3 {
		t.Fatalf("events = %#v", store.events)
	}
	want := []LifecycleEventKind{LifecycleAgentQueued, LifecycleAgentStarted, LifecycleAgentCompleted}
	for i, kind := range want {
		if store.events[i].Kind != kind || store.events[i].Version != uint64(i+1) {
			t.Fatalf("event[%d] = %#v", i, store.events[i])
		}
	}
}
