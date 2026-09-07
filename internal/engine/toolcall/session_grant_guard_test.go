package toolcall

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/tool"
)

func TestMutatingBashSessionGrantIsNotReused(t *testing.T) {
	handler := &fakeHandler{definition: tool.Definition{Name: "bash", Description: "fake shell", Kind: tool.KindBash, PermissionDetailKey: "command"}}
	promptCalls := 0
	service := newTestService(t, handler, permission.Config{}, WithPrompt(func(_ context.Context, req permission.Request) (permission.Resolution, error) {
		promptCalls++
		if req.Effect != tool.CommandEffectMutating {
			t.Fatalf("request effect = %q, want mutating", req.Effect)
		}
		return permission.Resolution{Action: permission.ActionAllow, Scope: permission.GrantScopeSession}, nil
	}))
	call, err := tool.NewCall("touch-1", "bash", json.RawMessage(`{"command":"touch file.go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Call(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	call.ID = "touch-2"
	if _, err := service.Call(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if promptCalls != 2 {
		t.Fatalf("prompt calls = %d, want 2", promptCalls)
	}
}

func TestUnknownBashEffectFailsClosedForSessionGrant(t *testing.T) {
	handler := &fakeHandler{definition: tool.Definition{Name: "bash", Description: "fake shell", Kind: tool.KindBash, PermissionDetailKey: "command"}}
	promptCalls := 0
	service := newTestService(t, handler, permission.Config{}, WithPrompt(func(_ context.Context, req permission.Request) (permission.Resolution, error) {
		promptCalls++
		if req.Effect != tool.CommandEffectUnknown || req.Risk != tool.CommandRiskDestructive {
			t.Fatalf("request = effect %q risk %q, want unknown/destructive", req.Effect, req.Risk)
		}
		return permission.Resolution{Action: permission.ActionAllow, Scope: permission.GrantScopeSession}, nil
	}))
	call, err := tool.NewCall("unknown-1", "bash", json.RawMessage(`{"command":"python3 script.py"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Call(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	call.ID = "unknown-2"
	if _, err := service.Call(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if promptCalls != 2 {
		t.Fatalf("prompt calls = %d, want 2", promptCalls)
	}
}
