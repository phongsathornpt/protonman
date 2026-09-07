package todotool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/projectTHORN/proton/internal/core/permission"
	tododomain "github.com/projectTHORN/proton/internal/feature/todo"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/adapter/out/tool/builtin"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
)

func TestTodoToolsValidateStructuredOutputThroughService(t *testing.T) {
	store, err := tododomain.NewStore([]tododomain.Item{{ID: "a", Text: "inspect", Status: tododomain.StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := builtin.NewRegistry(NewGetTodo(store), NewUpdateTodo(store))
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

	getCall, _ := tool.NewCall("todo-get", "get_todo", json.RawMessage(`{}`))
	getResult, err := service.Call(context.Background(), getCall)
	if err != nil {
		t.Fatalf("get_todo through service: %v", err)
	}
	if len(getResult.StructuredOutput) == 0 {
		t.Fatal("get_todo structured output is empty")
	}

	updateCall, _ := tool.NewCall("todo-update", "update_todo", todoPatchArgs(0,
		map[string]any{"op": "set_status", "id": "a", "status": "in_progress"},
	))
	updateResult, err := service.Call(context.Background(), updateCall)
	if err != nil {
		t.Fatalf("update_todo through service: %v", err)
	}
	if len(updateResult.StructuredOutput) == 0 {
		t.Fatal("update_todo structured output is empty")
	}
}

func TestGetTodoEmptySnapshotValidatesStructuredOutputThroughService(t *testing.T) {
	store, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := builtin.NewRegistry(NewGetTodo(store))
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
	call, _ := tool.NewCall("todo-empty", "get_todo", json.RawMessage(`{}`))
	result, err := service.Call(context.Background(), call)
	if err != nil {
		t.Fatalf("empty get_todo through service: %v", err)
	}
	if got := string(result.StructuredOutput); got != `{"revision":0,"items":[]}` {
		t.Fatalf("structured output = %s, want empty items array", got)
	}
}
