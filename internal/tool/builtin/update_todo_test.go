package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
)

func TestUpdateTodoReplacesSnapshot(t *testing.T) {
	store, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewUpdateTodo(store)
	args, _ := json.Marshal(map[string]any{"expected_revision": uint64(0), "items": []map[string]any{
		{"id": "a", "text": "inspect", "status": "completed"},
		{"id": "b", "text": "fix", "status": "in_progress"},
	}})
	call, err := tool.NewCall("todo-1", "update_todo", args)
	if err != nil {
		t.Fatal(err)
	}
	result, err := h.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, `"completed":1`) || !strings.Contains(result.Output, `"in_progress":1`) {
		t.Fatalf("output = %s", result.Output)
	}
	got := store.Snapshot()
	if got.Revision != 1 || len(got.Items) != 2 || got.Items[1].Status != tododomain.StatusInProgress {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestUpdateTodoRejectsDuplicateIDs(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := NewUpdateTodo(store)
	args := json.RawMessage(`{"expected_revision":0,"items":[{"id":"a","text":"one","status":"pending"},{"id":"a","text":"two","status":"pending"}]}`)
	call, _ := tool.NewCall("todo-1", "update_todo", args)
	if _, err := h.Execute(context.Background(), call); err == nil {
		t.Fatal("expected duplicate id error")
	}
	if len(store.Snapshot().Items) != 0 {
		t.Fatal("invalid update mutated store")
	}
}

func TestUpdateTodoDefinition(t *testing.T) {
	def := NewUpdateTodo(nil).Definition()
	if def.Kind != tool.KindTask || def.Mutability != tool.MutabilityMutating {
		t.Fatalf("definition = %#v", def)
	}
	if err := def.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateTodoRejectsStaleRevision(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := NewUpdateTodo(store)
	first, _ := json.Marshal(map[string]any{"expected_revision": uint64(0), "items": []map[string]any{{"id": "a", "text": "one", "status": "pending"}}})
	call, _ := tool.NewCall("todo-first", "update_todo", first)
	if _, err := h.Execute(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	stale, _ := json.Marshal(map[string]any{"expected_revision": uint64(0), "items": []map[string]any{{"id": "b", "text": "two", "status": "pending"}}})
	call, _ = tool.NewCall("todo-stale", "update_todo", stale)
	_, err := h.Execute(context.Background(), call)
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeConflict {
		t.Fatalf("error = %v, want conflict", err)
	}
	got := store.Snapshot()
	if got.Revision != 1 || len(got.Items) != 1 || got.Items[0].ID != "a" {
		t.Fatalf("stale update mutated store: %#v", got)
	}
}

func TestUpdateTodoRequiresExpectedRevision(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := NewUpdateTodo(store)
	args := json.RawMessage(`{"items":[]}`)
	call, _ := tool.NewCall("todo-missing-revision", "update_todo", args)
	_, err := h.Execute(context.Background(), call)
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeInvalidArguments {
		t.Fatalf("error=%v, want invalid arguments", err)
	}
}
