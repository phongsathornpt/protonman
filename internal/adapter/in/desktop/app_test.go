package desktop

import (
	"context"
	"testing"
	"time"
)

func TestControllerSnapshotAndSubscription(t *testing.T) {
	controller := NewController()
	defer controller.Close()

	if got := controller.Snapshot(context.Background()).Status; got != "Ready to connect" {
		t.Fatalf("initial status = %q", got)
	}

	updates, unsubscribe := controller.Subscribe(context.Background())
	controller.updateStatus("ready", "disconnected")

	select {
	case got := <-updates:
		if got.Status != "ready" {
			t.Fatalf("update status = %q", got.Status)
		}
	case <-context.Background().Done():
		t.Fatal("unreachable")
	}

	unsubscribe()
	controller.updateStatus("closed", "disconnected")
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

func TestControllerSetConfigOptionDisconnected(t *testing.T) {
	controller := NewController()
	defer controller.Close()

	ctx := context.Background()
	_, err := controller.SetConfigOption(ctx, "session-1", "provider", "anthropic")
	if err != ErrNotConnected {
		t.Fatalf("SetConfigOption error = %v, want ErrNotConnected", err)
	}

	_, err = controller.SetProvider(ctx, "session-1", "anthropic")
	if err != ErrNotConnected {
		t.Fatalf("SetProvider error = %v, want ErrNotConnected", err)
	}
}

func TestControllerStoreConfigOptions(t *testing.T) {
	controller := NewController()
	defer controller.Close()

	options := []sessionConfigOption{
		{
			ID: "provider",
			Options: []ModelOptionView{
				{Value: "openai", Name: "OpenAI"},
				{Value: "anthropic", Name: "Anthropic"},
			},
		},
		{
			ID: "model",
			Options: []ModelOptionView{
				{Value: "gpt-4o", Name: "GPT-4o"},
			},
			Error: "model discovery warning",
		},
	}

	controller.storeConfigOptions("s1", options)

	controller.mu.RLock()
	providers := controller.providerOptions["s1"]
	models := controller.modelOptions["s1"]
	modelErr := controller.modelOptionsError["s1"]
	controller.mu.RUnlock()

	if len(providers) != 2 || providers[0].Value != "openai" || providers[1].Value != "anthropic" {
		t.Fatalf("providerOptions = %#v", providers)
	}
	if len(models) != 1 || models[0].Value != "gpt-4o" {
		t.Fatalf("modelOptions = %#v", models)
	}
	if modelErr != "model discovery warning" {
		t.Fatalf("modelOptionsError = %q, want 'model discovery warning'", modelErr)
	}
}
