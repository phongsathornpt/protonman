package toolcall

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
)

type fakeHandler struct {
	definition       tool.Definition
	calls            int
	err              error
	waitForContext   bool
	shouldPanic      bool
	structuredOutput json.RawMessage
}

func (h *fakeHandler) Definition() tool.Definition {
	return h.definition
}

func (h *fakeHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	h.calls++
	result := tool.Result{
		CallID:           call.ID,
		ToolName:         call.Name,
		Output:           "executed",
		StructuredOutput: append(json.RawMessage(nil), h.structuredOutput...),
	}
	if h.shouldPanic {
		panic("handler secret")
	}
	if h.waitForContext {
		<-ctx.Done()
		return result, ctx.Err()
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

func TestCallDenyModeOverridesSessionGrant(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
	}
	service := newTestService(t, handler, permission.Config{}, WithPrompt(func(context.Context, permission.Request) (permission.Resolution, error) {
		return permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeSession,
		}, nil
	}))

	if _, err := service.Call(context.Background(), testCall(t)); err != nil {
		t.Fatalf("first Call() error = %v", err)
	}
	if err := service.SetMode(permission.ModeDeny); err != nil {
		t.Fatalf("SetMode() error = %v", err)
	}
	result, err := service.Call(context.Background(), testCall(t))
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Call() error = %v, want permission denied", err)
	}
	if !result.Denied {
		t.Fatal("Call() result.Denied = false, want true")
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", handler.calls)
	}
}

func TestCallPropagatesPermissionPromptCancellation(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
	}
	service := newTestService(t, handler, permission.Config{}, WithPrompt(func(context.Context, permission.Request) (permission.Resolution, error) {
		return permission.Resolution{}, context.Canceled
	}))

	result, err := service.Call(context.Background(), testCall(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Call() error = %v, want context canceled", err)
	}
	if result.Failure == nil || result.Failure.Code != tool.ErrorCodeCanceled {
		t.Fatalf("Call() failure = %#v, want canceled", result.Failure)
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls = %d, want 0", handler.calls)
	}
}

func TestCallAppliesExecutionTimeout(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
		waitForContext: true,
	}
	service := newTestService(t, handler, permission.Config{
		Rules: []permission.Rule{{
			Action: permission.ActionAllow,
			Tool:   permission.ToolBash,
		}},
	}, WithExecutionTimeout(20*time.Millisecond))

	result, err := service.Call(context.Background(), testCall(t))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call() error = %v, want deadline exceeded", err)
	}
	if result.Failure == nil || result.Failure.Code != tool.ErrorCodeDeadlineExceeded {
		t.Fatalf("Call() failure = %#v, want deadline_exceeded", result.Failure)
	}
}

func TestCallStartsExecutionTimeoutAfterPermission(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
		waitForContext: true,
	}
	permissionDelay := 30 * time.Millisecond
	service := newTestService(t, handler, permission.Config{},
		WithPermissionTimeout(200*time.Millisecond),
		WithExecutionTimeout(20*time.Millisecond),
		WithPrompt(func(ctx context.Context, _ permission.Request) (permission.Resolution, error) {
			select {
			case <-time.After(permissionDelay):
				return permission.Resolution{Action: permission.ActionAllow}, nil
			case <-ctx.Done():
				return permission.Resolution{}, ctx.Err()
			}
		}),
	)

	startedAt := time.Now()
	result, err := service.Call(context.Background(), testCall(t))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call() error = %v, want execution deadline exceeded", err)
	}
	if result.Failure == nil || result.Failure.Code != tool.ErrorCodeDeadlineExceeded {
		t.Fatalf("Call() failure = %#v, want deadline_exceeded", result.Failure)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls = %d, want 1 after permission", handler.calls)
	}
	if elapsed := time.Since(startedAt); elapsed < permissionDelay {
		t.Fatalf("Call() completed in %s, want permission delay to elapse", elapsed)
	}
}

func TestCallAppliesPermissionTimeoutBeforeHandler(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:                "bash",
			Description:         "fake shell",
			Kind:                tool.KindBash,
			PermissionDetailKey: "command",
		},
	}
	service := newTestService(t, handler, permission.Config{},
		WithPermissionTimeout(20*time.Millisecond),
		WithPrompt(func(ctx context.Context, _ permission.Request) (permission.Resolution, error) {
			<-ctx.Done()
			return permission.Resolution{}, ctx.Err()
		}),
	)

	result, err := service.Call(context.Background(), testCall(t))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call() error = %v, want permission deadline exceeded", err)
	}
	if result.Failure == nil || result.Failure.Code != tool.ErrorCodeDeadlineExceeded {
		t.Fatalf("Call() failure = %#v, want deadline_exceeded", result.Failure)
	}
	if handler.calls != 0 {
		t.Fatalf("handler calls = %d, want 0 before permission completes", handler.calls)
	}
}

func TestCallRecoversHandlerPanic(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name:        "bash",
			Description: "fake shell",
			Kind:        tool.KindBash,
		},
		shouldPanic: true,
	}
	service := newTestService(t, handler, permission.Config{
		Rules: []permission.Rule{{
			Action: permission.ActionAllow,
			Tool:   permission.ToolBash,
		}},
	})

	result, err := service.Call(context.Background(), testCall(t))
	if err == nil {
		t.Fatal("Call() error = nil, want recovered handler failure")
	}
	if got, want := err.Error(), "execute bash: tool handler panicked"; got != want {
		t.Fatalf("Call() error = %q, want %q", got, want)
	}
	if result.Failure == nil || result.Failure.Code != tool.ErrorCodeExecution {
		t.Fatalf("Call() failure = %#v, want execution_error", result.Failure)
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

func TestCloneDoesNotCopySessionGrants(t *testing.T) {
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
		t.Fatalf("Call() error = %v", err)
	}
	clone := service.Clone()
	if clone == nil {
		t.Fatal("Clone() returned nil")
	}
	if _, err := clone.Call(context.Background(), testCall(t)); err != nil {
		t.Fatalf("clone Call() error = %v", err)
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

func TestTaskMetadataAutoAllowedInAskButDeniedInDenyMode(t *testing.T) {
	handler := &fakeHandler{definition: tool.Definition{Name: "update_todo", Description: "update tasks", Kind: tool.KindTask, Mutability: tool.MutabilityMutating}}
	call, err := tool.NewCall("todo-1", "update_todo", json.RawMessage(`{"items":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	prompted := 0
	service := newTestService(t, handler, permission.Config{}, WithMode(permission.ModeAsk), WithPrompt(func(context.Context, permission.Request) (permission.Resolution, error) {
		prompted++
		return permission.Resolution{Action: permission.ActionAllow}, nil
	}))
	if _, err := service.Call(context.Background(), call); err != nil {
		t.Fatalf("ask mode task call: %v", err)
	}
	if prompted != 0 {
		t.Fatalf("task metadata prompted %d times, want 0", prompted)
	}
	if handler.calls != 1 {
		t.Fatalf("handler calls = %d", handler.calls)
	}
	if err := service.SetMode(permission.ModeDeny); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Call(context.Background(), call); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("deny mode error = %v", err)
	}
}

type slowCallerBoundedHandler struct{}

func (slowCallerBoundedHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                   "delegate_task",
		Description:            "long-running orchestration",
		Kind:                   tool.KindRead,
		Mutability:             tool.MutabilityMutating,
		ExecutionTimeoutPolicy: tool.ExecutionTimeoutCallerBounded,
	}
}

func (slowCallerBoundedHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	select {
	case <-time.After(30 * time.Millisecond):
		return tool.Result{CallID: call.ID, ToolName: call.Name, Output: "done"}, nil
	case <-ctx.Done():
		return tool.Result{}, ctx.Err()
	}
}

type singleHandlerRegistry struct{ handler tool.Handler }

func (r singleHandlerRegistry) Lookup(name string) (tool.Handler, bool) {
	if r.handler == nil || r.handler.Definition().Name != name {
		return nil, false
	}
	return r.handler, true
}
func (r singleHandlerRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.Definition()}
}

func TestCallerBoundedToolBypassesGenericExecutionTimeout(t *testing.T) {
	policy, err := permission.NewPolicy(permission.Config{Default: permission.ActionAllow})
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(singleHandlerRegistry{handler: slowCallerBoundedHandler{}}, policy, WithExecutionTimeout(10*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	call, err := tool.NewCall("delegate-1", "delegate_task", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := service.Call(ctx, call); err != nil {
		t.Fatalf("Call() error = %v, caller-bounded tool should not use service timeout", err)
	}
	if elapsed := time.Since(started); elapsed < 25*time.Millisecond {
		t.Fatalf("Call() elapsed = %v, test did not exceed generic timeout", elapsed)
	}
}

func TestServiceValidatesStructuredOutputSchema(t *testing.T) {
	handler := &fakeHandler{
		definition: tool.Definition{
			Name: "structured", Description: "structured output", Kind: tool.KindRead,
			OutputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"count": map[string]any{"type": "number"}},
				"required":   []any{"count"},
			},
		},
		structuredOutput: json.RawMessage(`{"count":3}`),
	}
	service := newTestService(t, handler, permission.Config{Rules: []permission.Rule{{Action: permission.ActionAllow, Tool: permission.ToolRead}}})
	call, err := tool.NewCall("call-structured", "structured", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Call(context.Background(), call); err != nil {
		t.Fatalf("Call() error = %v", err)
	}

	handler.structuredOutput = json.RawMessage(`{"count":"three"}`)
	result, err := service.Call(context.Background(), call)
	if err == nil {
		t.Fatal("Call() error = nil, want output schema failure")
	}
	if result.Failure == nil || result.Failure.Code != tool.ErrorCodeInvalidOutput {
		t.Fatalf("failure = %#v, want invalid_output", result.Failure)
	}
}
