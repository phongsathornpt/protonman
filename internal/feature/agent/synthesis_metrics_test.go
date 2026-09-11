package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

func TestSynthesisMetricsMeasureDeliveryAndSuppression(t *testing.T) {
	var mu sync.Mutex
	seen := make([]MetricEvent, 0)
	coord := NewCoordinator(nil, nil, nil, nil,
		WithMetricObserver(func(_ context.Context, event MetricEvent) {
			mu.Lock()
			seen = append(seen, event)
			mu.Unlock()
		}),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return &mockRunner{runFunc: func(context.Context, []model.Message, turn.Sink) (turn.Result, error) {
				return turn.Result{Message: model.Message{Role: model.RoleAssistant, Content: "bounded finding"}}, nil
			}}, nil
		}),
	)
	defer coord.Close()

	ref := TurnRef{SessionID: "session-metrics", TurnID: "turn-metrics"}
	handle, err := coord.Spawn(context.Background(), Request{
		SessionID: ref.SessionID, ParentID: ref.TurnID, Profile: ProfileAgility, Task: "inspect",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coord.Wait(context.Background(), handle.ID, time.Second); err != nil {
		t.Fatal(err)
	}

	synth := NewSynthesisCoordinator(coord)
	batch, err := synth.DrainReady(ref)
	if err != nil || len(batch.Results) != 1 {
		t.Fatalf("first drain=%+v err=%v", batch, err)
	}
	synth.MarkConsumed(context.Background(), batch)
	synth.MarkConsumed(context.Background(), batch)
	batch, err = synth.DrainReady(ref)
	if err != nil || len(batch.Results) != 0 {
		t.Fatalf("second drain=%+v err=%v", batch, err)
	}

	if _, err := coord.WaitActivityForTurn(context.Background(), ref, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	events := append([]MetricEvent(nil), seen...)
	mu.Unlock()
	assertSynthesisMetricSet(t, events)
}

func assertSynthesisMetricSet(t *testing.T, events []MetricEvent) {
	t.Helper()
	bytesByKind := map[MetricKind]int64{}
	countByKind := map[MetricKind]int{}
	for _, event := range events {
		bytesByKind[event.Kind] += event.Bytes
		countByKind[event.Kind] += event.Count
	}
	for _, kind := range []MetricKind{MetricResultBytes, MetricResultConsumedBytes, MetricDuplicateResultBytes, MetricWaitSnapshotBytes} {
		if bytesByKind[kind] <= 0 {
			t.Fatalf("metric %s bytes=%d events=%#v", kind, bytesByKind[kind], events)
		}
	}
	if countByKind[MetricSynthesisAgents] != 1 {
		t.Fatalf("synthesis agents=%d events=%#v", countByKind[MetricSynthesisAgents], events)
	}
}
