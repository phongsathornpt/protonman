package todotool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

func todoCapabilityService(t *testing.T, items []tododomain.Item) *toolcall.Service {
	t.Helper()
	store, err := tododomain.NewStore(items)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := builtin.NewRegistry(NewTodo(store))
	if err != nil {
		t.Fatal(err)
	}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(permission.ModeAlwaysApprove))
	if err != nil {
		t.Fatal(err)
	}
	return service
}

// These wire variants previously failed at the tool-call boundary with
// "tool \"todo\" arguments do not match its input schema", which the model could
// not act on because the actionable clause was also truncated.
func TestTodoCapabilityServiceAcceptsWireVariants(t *testing.T) {
	calls := map[string]string{
		"canonical update":  `{"action":"update","expectedRevision":0,"operations":[{"op":"set_status","id":"a","status":"in_progress"}]}`,
		"quoted revision":   `{"action":"update","expectedRevision":"0","operations":[{"op":"set_status","id":"a","status":"in_progress"}]}`,
		"echoed session_id": `{"action":"update","sessionId":"session-1","expectedRevision":0,"operations":[{"op":"set_status","id":"a","status":"in_progress"}]}`,
		"uppercase op":      `{"action":"update","expectedRevision":0,"operations":[{"op":"SET_STATUS","id":"a","status":"in_progress"}]}`,
		"uppercase status":  `{"action":"update","expectedRevision":0,"operations":[{"op":"set_status","id":"a","status":"IN_PROGRESS"}]}`,
		"echoed get args":   `{"action":"get","expectedRevision":0,"sessionId":"session-1"}`,
	}
	for name, args := range calls {
		t.Run(name, func(t *testing.T) {
			service := todoCapabilityService(t, []tododomain.Item{{ID: "a", Text: "inspect", Status: tododomain.StatusPending}})
			call, err := tool.NewCall("todo-call", "todo", json.RawMessage(args))
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Call(context.Background(), call)
			if err != nil {
				t.Fatalf("call %s failed: %v", args, err)
			}
			if result.Failure != nil {
				t.Fatalf("call %s returned failure: %#v", args, result.Failure)
			}
		})
	}
}

// Header-only case-variant calls must still reach the same handler, proving the
// published enum is no stricter than dispatch.
func TestTodoCapabilityNormalizesUppercaseAction(t *testing.T) {
	service := todoCapabilityService(t, nil)
	call, err := tool.NewCall("todo-call", "todo", json.RawMessage(`{"action":"GET"}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Call(context.Background(), call)
	if err != nil {
		t.Fatalf("uppercase action failed: %v", err)
	}
	if result.Failure != nil {
		t.Fatalf("uppercase action returned failure: %#v", result.Failure)
	}
}

func TestTodoCapabilityServiceRejectsGenuineMisuse(t *testing.T) {
	calls := map[string]string{
		"missing revision":  `{"action":"update","operations":[{"op":"remove","id":"a"}]}`,
		"non-numeric":       `{"action":"update","expectedRevision":"abc","operations":[{"op":"remove","id":"a"}]}`,
		"unknown op":        `{"action":"update","expectedRevision":0,"operations":[{"op":"nope","id":"a"}]}`,
		"unknown action":    `{"action":"delete"}`,
		"get with ops":      `{"action":"get","operations":[{"op":"remove","id":"a"}]}`,
		"unknown top-level": `{"action":"get","bogus":1}`,
	}
	for name, args := range calls {
		t.Run(name, func(t *testing.T) {
			service := todoCapabilityService(t, nil)
			call, err := tool.NewCall("todo-call", "todo", json.RawMessage(args))
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Call(context.Background(), call)
			if err == nil || result.Failure == nil {
				t.Fatalf("call %s was accepted, want invalid arguments", args)
			}
			if result.Failure.Code != tool.ErrorCodeInvalidArguments {
				t.Fatalf("call %s failure code = %q, want invalid arguments", args, result.Failure.Code)
			}
		})
	}
}

// A misconfigured revision must be attributable: the model-visible failure has to
// name the offending field after compaction.
func TestTodoCapabilityFailureNamesOffendingField(t *testing.T) {
	service := todoCapabilityService(t, nil)
	call, err := tool.NewCall("todo-call", "todo", json.RawMessage(`{"action":"update","operations":[{"op":"remove","id":"a"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Call(context.Background(), call)
	if err == nil {
		t.Fatal("missing expected_revision was accepted")
	}
	payload := result.ModelPayload()
	if payload.Failure == nil {
		t.Fatal("model payload lost the failure")
	}
	visible := payload.Failure.Message + " " + payload.Failure.Diagnostic
	if !strings.Contains(visible, "expectedRevision") {
		t.Fatalf("model-visible failure does not name the field: %q", visible)
	}
}
