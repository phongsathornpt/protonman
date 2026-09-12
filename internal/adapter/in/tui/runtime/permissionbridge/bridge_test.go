package permissionbridge

import (
	"context"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestBridgePromptAndRespond(t *testing.T) {
	bridge := New()
	defer bridge.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		req := permission.Request{ToolName: "read", Detail: "read README"}
		res, err := bridge.Prompt(ctx, req)
		if err != nil {
			t.Errorf("Prompt error: %v", err)
		}
		if res.Action != permission.ActionAllow {
			t.Errorf("Resolution action = %v, want allow", res.Action)
		}
		close(done)
	}()

	msg := bridge.Next()()
	reqMsg, ok := msg.(RequestMsg)
	if !ok {
		t.Fatalf("expected RequestMsg, got %T", msg)
	}
	if reqMsg.Request.Request.ToolName != "read" {
		t.Fatalf("tool name = %q, want read", reqMsg.Request.Request.ToolName)
	}
	reqMsg.Request.Respond(permission.Resolution{Action: permission.ActionAllow})

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for response")
	}
}

func TestBridgeClose(t *testing.T) {
	bridge := New()
	bridge.Close()

	msg := bridge.Next()()
	if _, ok := msg.(ClosedMsg); !ok {
		t.Fatalf("expected ClosedMsg, got %T", msg)
	}

	_, err := bridge.Prompt(context.Background(), permission.Request{})
	if err == nil {
		t.Fatal("expected error from prompt after close")
	}
}
