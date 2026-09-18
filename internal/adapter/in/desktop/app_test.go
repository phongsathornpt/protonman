package desktop

import (
	"context"
	"testing"
	"time"
)

func TestControllerSnapshotAndSubscription(t *testing.T) {
	controller := NewController()
	defer controller.Close()

	if got := controller.Snapshot(context.Background()).Status; got != "Wails migration shell" {
		t.Fatalf("initial status = %q", got)
	}

	updates, unsubscribe := controller.Subscribe(context.Background())
	controller.SetStatus("ready")

	select {
	case got := <-updates:
		if got.Status != "ready" {
			t.Fatalf("update status = %q", got.Status)
		}
	case <-context.Background().Done():
		t.Fatal("unreachable")
	}

	unsubscribe()
	controller.SetStatus("closed")
	if _, ok := <-updates; ok {
		t.Fatal("updates channel remains open after unsubscribe")
	}
}

func TestControllerContextUnsubscribe(t *testing.T) {
	controller := NewController()
	defer controller.Close()

	ctx, cancel := context.WithCancel(context.Background())
	updates, _ := controller.Subscribe(ctx)
	cancel()

	select {
	case _, ok := <-updates:
		if ok {
			t.Fatal("updates channel remains open after context cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("context cancellation did not unsubscribe")
	}
}
