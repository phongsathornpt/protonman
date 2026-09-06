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

func TestUpdateTodoReportsStructuredChanges(t *testing.T) {
	store, _ := tododomain.NewStore([]tododomain.Item{
		{ID: "start", Text: "start", Status: tododomain.StatusPending},
		{ID: "complete", Text: "complete", Status: tododomain.StatusInProgress},
		{ID: "reopen", Text: "reopen", Status: tododomain.StatusCompleted},
		{ID: "remove", Text: "remove", Status: tododomain.StatusPending},
	})
	h := NewUpdateTodo(store)
	args, _ := json.Marshal(map[string]any{"expected_revision": uint64(0), "items": []map[string]any{
		{"id": "start", "text": "start", "status": "in_progress"},
		{"id": "complete", "text": "complete", "status": "completed"},
		{"id": "reopen", "text": "reopen", "status": "pending"},
		{"id": "add", "text": "add", "status": "pending"},
	}})
	call, _ := tool.NewCall("todo-diff", "update_todo", args)
	result, err := h.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"added":1`, `"removed":1`, `"started":1`, `"completed":1`, `"reopened":1`} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("output=%s missing %s", result.Output, want)
		}
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

func TestUpdateTodoPermissionDetailSummarizesChanges(t *testing.T) {
	store, _ := tododomain.NewStore([]tododomain.Item{
		{ID: "done", Text: "done", Status: tododomain.StatusInProgress},
		{ID: "remove", Text: "remove", Status: tododomain.StatusPending},
	})
	h := NewUpdateTodo(store).(updateTodoHandler)
	args, _ := json.Marshal(map[string]any{"expected_revision": uint64(0), "items": []map[string]any{
		{"id": "done", "text": "done", "status": "completed"},
		{"id": "add", "text": "add", "status": "pending"},
	}})
	got := h.PermissionDetail(args)
	for _, want := range []string{"1 completed", "1 added", "1 removed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("permission detail=%q missing %q", got, want)
		}
	}
}

func TestUpdateTodoPermissionDetailFlagsStaleRevision(t *testing.T) {
	store, _ := tododomain.NewStore([]tododomain.Item{{ID: "a", Text: "a", Status: tododomain.StatusPending}})
	if _, err := store.CompareAndReplace(context.Background(), 0, []tododomain.Item{{ID: "a", Text: "a", Status: tododomain.StatusInProgress}}); err != nil {
		t.Fatal(err)
	}
	h := NewUpdateTodo(store).(updateTodoHandler)
	args := json.RawMessage(`{"expected_revision":0,"items":[{"id":"a","text":"a","status":"completed"}]}`)
	if got := h.PermissionDetail(args); !strings.Contains(got, "stale task plan") {
		t.Fatalf("permission detail=%q, want stale task plan", got)
	}
}

func TestUpdateTodoPermissionDetailShowsNoChanges(t *testing.T) {
	items := []tododomain.Item{{ID: "a", Text: "a", Status: tododomain.StatusPending}}
	store, _ := tododomain.NewStore(items)
	h := NewUpdateTodo(store).(updateTodoHandler)
	args, _ := json.Marshal(map[string]any{"expected_revision": uint64(0), "items": items})
	if got := h.PermissionDetail(args); !strings.Contains(got, "no changes") {
		t.Fatalf("permission detail=%q, want no changes", got)
	}
}
