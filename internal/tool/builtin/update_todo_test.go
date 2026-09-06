package builtin

import (
	"context"
	"encoding/json"
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
	args, _ := json.Marshal(map[string]any{"items": []map[string]any{
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
	args := json.RawMessage(`{"items":[{"id":"a","text":"one","status":"pending"},{"id":"a","text":"two","status":"pending"}]}`)
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
