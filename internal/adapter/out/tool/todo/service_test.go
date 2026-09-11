package todotool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/tool/builtin"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

func todoCapabilityPatchArgs(revision uint64, operations ...map[string]any) json.RawMessage {
	payload, _ := json.Marshal(map[string]any{"action": "update", "expected_revision": revision, "operations": operations})
	return payload
}

func TestTodoToolsValidateStructuredOutputThroughService(t *testing.T) {
	store, err := tododomain.NewStore([]tododomain.Item{{ID: "a", Text: "inspect", Status: tododomain.StatusPending}})
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

	getCall, _ := tool.NewCall("todo-get", "todo", json.RawMessage(`{"action":"get"}`))
	getResult, err := service.Call(context.Background(), getCall)
	if err != nil {
		t.Fatalf("todo get through service: %v", err)
	}
	if len(getResult.StructuredOutput) == 0 {
		t.Fatal("todo get structured output is empty")
	}

	updateCall, _ := tool.NewCall("todo-update", "todo", todoCapabilityPatchArgs(0,
		map[string]any{"op": "set_status", "id": "a", "status": "in_progress"},
	))
	updateResult, err := service.Call(context.Background(), updateCall)
	if err != nil {
		t.Fatalf("todo update through service: %v", err)
	}
	if len(updateResult.StructuredOutput) == 0 {
		t.Fatal("todo update structured output is empty")
	}
}

func TestGetTodoEmptySnapshotValidatesStructuredOutputThroughService(t *testing.T) {
	store, err := tododomain.NewStore(nil)
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
	call, _ := tool.NewCall("todo-empty", "todo", json.RawMessage(`{"action":"get"}`))
	result, err := service.Call(context.Background(), call)
	if err != nil {
		t.Fatalf("empty todo get through service: %v", err)
	}
	if got := string(result.StructuredOutput); got != `{"revision":0,"items":[]}` {
		t.Fatalf("structured output = %s, want empty items array", got)
	}
}

func TestTodoServiceRejectsOperationFieldsOutsideSelectedOp(t *testing.T) {
	store, err := tododomain.NewStore([]tododomain.Item{{ID: "a", Text: "inspect", Status: tododomain.StatusPending}})
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
	call, _ := tool.NewCall("todo-invalid-shape", "todo", todoCapabilityPatchArgs(0, map[string]any{
		"op": "set_status", "id": "a", "status": "completed", "text": "forbidden",
	}))
	result, err := service.Call(context.Background(), call)
	if err == nil || result.Failure == nil || result.Failure.Code != tool.ErrorCodeInvalidArguments {
		t.Fatalf("result=%#v err=%v, want invalid arguments", result, err)
	}
	if result.Failure.Diagnostic == "" {
		t.Fatalf("missing schema diagnostic: %#v", result.Failure)
	}
}

func TestTodoServiceRejectsUpdateWithoutRevisionBeforeExecution(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	registry, err := builtin.NewRegistry(NewTodo(store))
	if err != nil {
		t.Fatal(err)
	}
	policy, _ := permission.NewPolicy(permission.Config{})
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(permission.ModeAlwaysApprove))
	if err != nil {
		t.Fatal(err)
	}
	call, _ := tool.NewCall("todo-missing-revision", "todo", json.RawMessage(`{"action":"update","operations":[{"op":"add","id":"a","text":"inspect","status":"pending"}]}`))
	result, err := service.Call(context.Background(), call)
	if err == nil || result.Failure == nil || result.Failure.Code != tool.ErrorCodeInvalidArguments {
		t.Fatalf("result=%#v err=%v, want invalid arguments", result, err)
	}
}

func TestTodoConflictRefreshesSnapshotWithoutMasqueradingAsUpdateSuccess(t *testing.T) {
	store, err := tododomain.NewStore([]tododomain.Item{{ID: "a", Text: "inspect", Status: tododomain.StatusPending}})
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
	first, _ := tool.NewCall("todo-first", "todo", todoCapabilityPatchArgs(0,
		map[string]any{"op": "set_status", "id": "a", "status": "in_progress"},
	))
	if _, err := service.Call(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	stale, _ := tool.NewCall("todo-stale", "todo", todoCapabilityPatchArgs(0,
		map[string]any{"op": "set_status", "id": "a", "status": "completed"},
	))
	result, err := service.Call(context.Background(), stale)
	if err == nil {
		t.Fatal("stale update unexpectedly succeeded")
	}
	if result.Failure == nil || result.Failure.Code != tool.ErrorCodeConflict {
		t.Fatalf("failure = %#v", result.Failure)
	}
	evidence := result.Failure.RecoveryEvidence
	if evidence == nil || evidence.Tool != "todo" || evidence.Action != tool.RecoveryRefreshResource {
		t.Fatalf("recovery evidence = %#v", evidence)
	}
	var snapshot struct {
		Revision uint64            `json:"revision"`
		Items    []tododomain.Item `json:"items"`
	}
	if err := json.Unmarshal(evidence.StructuredOutput, &snapshot); err != nil {
		t.Fatalf("decode recovery snapshot: %v", err)
	}
	if snapshot.Revision != 1 || len(snapshot.Items) != 1 || snapshot.Items[0].Status != tododomain.StatusInProgress {
		t.Fatalf("recovery snapshot = %#v", snapshot)
	}
	if store.Snapshot().Revision != 1 {
		t.Fatalf("stale update mutated store: %#v", store.Snapshot())
	}
}
