package toolcall

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
)

type fakeHandler struct {
	definition tool.Definition
	calls      int
	err        error
}

func (h *fakeHandler) Definition() tool.Definition {
	return h.definition
}

func (h *fakeHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls++
	result := tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   "executed",
	}
	return result, h.err
}

type fakeRegistry struct {
	handler *fakeHandler
}

func (r *fakeRegistry) Lookup(name string) (tool.Handler, bool) {
	if r.handler.definition.Name != name {
		return nil, false
	}
	return r.handler, true
}

func (r *fakeRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.definition}
}

func newTestService(t *testing.T, handler *fakeHandler, policyConfig permission.Config, options ...Option) *Service {
	t.Helper()
	policy, err := permission.NewPolicy(policyConfig)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := NewService(&fakeRegistry{handler: handler}, policy, options...)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	return service
}

func testCall(t *testing.T) tool.Call {
	t.Helper()
	call, err := tool.NewCall("call-1", "bash", json.RawMessage(`{"command":"printf ok"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	return call
}

func TestCallDeniedBeforeHandler(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
	}
	service := newTestService(t, handler, permission.Config{
		Rules: []permission.Rule{{
			Action:  permission.ActionDeny,
			Tool:    permission.ToolBash,
			Pattern: "printf *",
		}},
	})

	result, err := service.Call(context.Background(), testCall(t))
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Call() error = %v, want permission denied", err)
	}
	if !result.Denied {
		t.Fatal("Call() result.Denied = false, want true")
	}
	if result.Failure == nil || result.Failure.Code != tool.ErrorCodePermissionDenied {
		t.Fatalf("Call() failure = %#v, want permission_denied", result.Failure)
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls = %d, want 0", handler.calls)
	}
}

func TestCallPromptsAndExecutesOnce(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
	}
	promptCalls := 0
	service := newTestService(t, handler, permission.Config{}, WithPrompt(func(_ context.Context, request permission.Request) (permission.Resolution, error) {
		promptCalls++
		if request.Detail != "printf ok" {
			t.Fatalf("prompt detail = %q, want printf ok", request.Detail)
		}
		return permission.Resolution{
			Action: permission.ActionAllow,
			Reason: "test approval",
		}, nil
	}))

	result, err := service.Call(context.Background(), testCall(t))
	if err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if result.Output != "executed" {
		t.Fatalf("Call() output = %q, want executed", result.Output)
	}
	if promptCalls != 1 || handler.calls != 1 {
		t.Fatalf("prompt calls = %d, handler calls = %d, want 1 and 1", promptCalls, handler.calls)
	}
}

func TestCallPolicyAllowSkipsPrompt(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
	}
	service := newTestService(t, handler, permission.Config{
		Rules: []permission.Rule{{
			Action: permission.ActionAllow,
			Tool:   permission.ToolBash,
		}},
	}, WithPrompt(func(context.Context, permission.Request) (permission.Resolution, error) {
		t.Fatal("prompt called for statically allowed request")
		return permission.Resolution{}, nil
	}))

	if _, err := service.Call(context.Background(), testCall(t)); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", handler.calls)
	}
}

func TestCallSessionGrantDoesNotChangeMode(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
	}
	promptCalls := 0
	service := newTestService(t, handler, permission.Config{}, WithPrompt(func(context.Context, permission.Request) (permission.Resolution, error) {
		promptCalls++
		return permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeSession,
		}, nil
	}))

	if _, err := service.Call(context.Background(), testCall(t)); err != nil {
		t.Fatalf("first Call() error = %v", err)
	}
	if _, err := service.Call(context.Background(), testCall(t)); err != nil {
		t.Fatalf("second Call() error = %v", err)
	}
	if promptCalls != 1 {
		t.Fatalf("prompt calls = %d, want 1", promptCalls)
	}
	if service.Mode() != permission.ModeAsk {
		t.Fatalf("service mode = %s, want ask", service.Mode())
	}
}

func TestCallSessionGrantIsNarrowToExactRequest(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
	}
	promptCalls := 0
	service := newTestService(t, handler, permission.Config{}, WithPrompt(func(context.Context, permission.Request) (permission.Resolution, error) {
		promptCalls++
		return permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeSession,
		}, nil
	}))

	if _, err := service.Call(context.Background(), testCall(t)); err != nil {
		t.Fatalf("first Call() error = %v", err)
	}
	otherCall, err := tool.NewCall("call-2", "bash", json.RawMessage(`{"command":"printf other"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	if _, err := service.Call(context.Background(), otherCall); err != nil {
		t.Fatalf("second Call() error = %v", err)
	}
	if promptCalls != 2 {
		t.Fatalf("prompt calls = %d, want 2", promptCalls)
	}
}

func TestCallPropagatesStructuredHandlerFailure(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "read_file",
			Description:         "fake reader",
			Kind:                tool.KindRead,
			PermissionDetailKey: "path",
		},
		err: tool.WrapToolError(tool.ErrorCodeNotFound, "read target missing", errors.New("no such file")),
	}
	service := newTestService(t, handler, permission.Config{
		Rules: []permission.Rule{{
			Action: permission.ActionAllow,
			Tool:   permission.ToolRead,
		}},
	})
	call, err := tool.NewCall("call-1", "read_file", json.RawMessage(`{"path":"missing.txt"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}

	result, err := service.Call(context.Background(), call)
	if err == nil {
		t.Fatal("Call() error = nil, want handler error")
	}
	if result.Failure == nil || result.Failure.Code != tool.ErrorCodeNotFound {
		t.Fatalf("Call() failure = %#v, want not_found", result.Failure)
	}
}

type fakeDetailedHandler struct {
	fakeHandler
	customDetail string
}

func (h *fakeDetailedHandler) PermissionDetail(_ json.RawMessage) string {
	return h.customDetail
}

func TestCallUsesDetailProvider(t *testing.T) {
	handler := &fakeDetailedHandler{
		fakeHandler: fakeHandler{
			definition: tool.Definition{
				Name:                "apply_patch",
				Description:         "fake patch",
				Kind:                tool.KindEdit,
				PermissionDetailKey: "patch",
			},
		},
		customDetail: "main.go",
	}
	policyConfig := permission.Config{
		Rules: []permission.Rule{{
			Action:  permission.ActionAllow,
			Tool:    permission.ToolEdit,
			Pattern: "*.go",
		}},
	}
	policy, err := permission.NewPolicy(policyConfig)
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}

	// Supply handler via a registry that returns fakeDetailedHandler
	call, err := tool.NewCall("call-patch", "apply_patch", json.RawMessage(`{"patch":"*** Begin Patch\n*** End Patch"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}

	customRegistry := &customHandlerRegistry{handler: handler}
	serviceWithCustom, err := NewService(customRegistry, policy)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := serviceWithCustom.Call(context.Background(), call)
	if err != nil {
		t.Fatalf("Call() error = %v, want allow from custom detail matching *.go", err)
	}
	if result.Output != "executed" {
		t.Fatalf("result output = %q, want executed", result.Output)
	}
}

type customHandlerRegistry struct {
	handler tool.Handler
}

func (r *customHandlerRegistry) Lookup(name string) (tool.Handler, bool) {
	if r.handler.Definition().Name != name {
		return nil, false
	}
	return r.handler, true
}

func (r *customHandlerRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.Definition()}
}
