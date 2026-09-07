package todotool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/core/tool"
	tododomain "github.com/projectTHORN/proton/internal/feature/todo"
)

func todoPatchArgs(revision uint64, operations ...map[string]any) json.RawMessage {
	payload, _ := json.Marshal(map[string]any{"expected_revision": revision, "operations": operations})
	return payload
}

func TestUpdateTodoPatchesSnapshotWithoutOmissionDeletes(t *testing.T) {
	store, err := tododomain.NewStore([]tododomain.Item{{ID: "keep", Text: "keep", Status: tododomain.StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	h := NewUpdateTodo(store)
	call, _ := tool.NewCall("todo-1", "update_todo", todoPatchArgs(0,
		map[string]any{"op": "add", "id": "a", "text": "inspect", "status": "completed"},
		map[string]any{"op": "add", "id": "b", "text": "fix", "status": "in_progress"},
	))
	result, err := h.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "2 added") {
		t.Fatalf("output = %s", result.Output)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.StructuredOutput, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["total"] != float64(3) || payload["completed"] != float64(1) || payload["in_progress"] != float64(1) {
		t.Fatalf("structured output = %#v", payload)
	}
	got := store.Snapshot()
	if got.Revision != 1 || len(got.Items) != 3 || got.Items[0].ID != "keep" || got.Items[2].Status != tododomain.StatusInProgress {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestUpdateTodoRejectsDuplicateAddID(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := NewUpdateTodo(store)
	call, _ := tool.NewCall("todo-1", "update_todo", todoPatchArgs(0,
		map[string]any{"op": "add", "id": "a", "text": "one", "status": "pending"},
		map[string]any{"op": "add", "id": "a", "text": "two", "status": "pending"},
	))
	if _, err := h.Execute(context.Background(), call); err == nil {
		t.Fatal("expected duplicate id error")
	}
	if len(store.Snapshot().Items) != 0 {
		t.Fatal("invalid patch mutated store")
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
	call, _ := tool.NewCall("todo-diff", "update_todo", todoPatchArgs(0,
		map[string]any{"op": "set_status", "id": "start", "status": "in_progress"},
		map[string]any{"op": "set_status", "id": "complete", "status": "completed"},
		map[string]any{"op": "set_status", "id": "reopen", "status": "pending"},
		map[string]any{"op": "remove", "id": "remove"},
		map[string]any{"op": "add", "id": "add", "text": "add", "status": "pending"},
	))
	result, err := h.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	body := string(result.StructuredOutput)
	for _, want := range []string{`"added":1`, `"removed":1`, `"started":1`, `"completed":1`, `"reopened":1`} {
		if !strings.Contains(body, want) {
			t.Fatalf("structured output=%s missing %s", body, want)
		}
	}
}

func TestUpdateTodoDefinitionUsesPatchSchema(t *testing.T) {
	def := NewUpdateTodo(nil).Definition()
	if def.Kind != tool.KindTask || def.Mutability != tool.MutabilityMutating || len(def.OutputSchema) == 0 {
		t.Fatalf("definition = %#v", def)
	}
	props := def.InputSchema["properties"].(map[string]any)
	if _, ok := props["operations"]; !ok {
		t.Fatal("operations schema missing")
	}
	if _, legacy := props["items"]; legacy {
		t.Fatal("legacy items replacement remains exposed")
	}
	operations := props["operations"].(map[string]any)
	items := operations["items"].(map[string]any)
	oneOf, ok := items["oneOf"].([]any)
	if !ok || len(oneOf) != 4 {
		t.Fatalf("operation schema oneOf = %#v, want four exact operation shapes", items["oneOf"])
	}
	if err := def.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateTodoReplaysStaleRevisionWhenPatchStillApplies(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := NewUpdateTodo(store)
	first, _ := tool.NewCall("todo-first", "update_todo", todoPatchArgs(0, map[string]any{"op": "add", "id": "a", "text": "one", "status": "pending"}))
	if _, err := h.Execute(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	stale, _ := tool.NewCall("todo-stale", "update_todo", todoPatchArgs(0, map[string]any{"op": "add", "id": "b", "text": "two", "status": "pending"}))
	if _, err := h.Execute(context.Background(), stale); err != nil {
		t.Fatalf("stale replay error = %v", err)
	}
	got := store.Snapshot()
	if got.Revision != 2 || len(got.Items) != 2 || got.Items[0].ID != "a" || got.Items[1].ID != "b" {
		t.Fatalf("replayed snapshot: %#v", got)
	}
}

func TestUpdateTodoStaleReplayReturnsStructuredRefreshWhenPatchConflicts(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := NewUpdateTodo(store)
	first, _ := tool.NewCall("todo-first", "update_todo", todoPatchArgs(0, map[string]any{"op": "add", "id": "a", "text": "one", "status": "pending"}))
	if _, err := h.Execute(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	stale, _ := tool.NewCall("todo-stale", "update_todo", todoPatchArgs(0, map[string]any{"op": "add", "id": "a", "text": "two", "status": "pending"}))
	_, err := h.Execute(context.Background(), stale)
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeConflict {
		t.Fatalf("error = %v, want conflict", err)
	}
	if toolErr.Recovery == nil || toolErr.Recovery.Action != "refresh_resource" || toolErr.Recovery.Tool != "get_todo" || string(toolErr.Recovery.Arguments) != `{}` {
		t.Fatalf("recovery = %#v", toolErr.Recovery)
	}
}

func TestUpdateTodoRequiresExpectedRevisionAndOperations(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := NewUpdateTodo(store)
	for _, args := range []json.RawMessage{
		json.RawMessage(`{"operations":[{"op":"add","id":"a","text":"a","status":"pending"}]}`),
		json.RawMessage(`{"expected_revision":0,"operations":[]}`),
	} {
		call, _ := tool.NewCall("todo-invalid", "update_todo", args)
		_, err := h.Execute(context.Background(), call)
		var toolErr *tool.ToolError
		if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeInvalidArguments {
			t.Fatalf("error=%v, want invalid arguments", err)
		}
	}
}

func TestUpdateTodoRejectsLegacySnapshotAndUnknownFields(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := NewUpdateTodo(store)
	for _, args := range []json.RawMessage{
		json.RawMessage(`{"expected_revision":0,"items":[]}`),
		json.RawMessage(`{"expected_revision":0,"operations":[{"op":"add","id":"a","text":"a","status":"pending","banana":true}]}`),
	} {
		call, _ := tool.NewCall("todo-legacy", "update_todo", args)
		if _, err := h.Execute(context.Background(), call); err == nil {
			t.Fatalf("arguments %s unexpectedly accepted", args)
		}
	}
}

func TestUpdateTodoPermissionDetailSummarizesPatch(t *testing.T) {
	store, _ := tododomain.NewStore([]tododomain.Item{
		{ID: "done", Text: "done", Status: tododomain.StatusInProgress},
		{ID: "remove", Text: "remove", Status: tododomain.StatusPending},
	})
	h := NewUpdateTodo(store).(updateTodoHandler)
	args := todoPatchArgs(0,
		map[string]any{"op": "set_status", "id": "done", "status": "completed"},
		map[string]any{"op": "remove", "id": "remove"},
		map[string]any{"op": "add", "id": "add", "text": "add", "status": "pending"},
	)
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
	args := todoPatchArgs(0, map[string]any{"op": "set_status", "id": "a", "status": "completed"})
	if got := h.PermissionDetail(args); !strings.Contains(got, "stale task patch") {
		t.Fatalf("permission detail=%q, want stale task patch", got)
	}
}

func TestUpdateTodoPermissionDetailShowsNoChanges(t *testing.T) {
	items := []tododomain.Item{{ID: "a", Text: "a", Status: tododomain.StatusPending}}
	store, _ := tododomain.NewStore(items)
	h := NewUpdateTodo(store).(updateTodoHandler)
	args := todoPatchArgs(0, map[string]any{"op": "set_status", "id": "a", "status": "pending"})
	if got := h.PermissionDetail(args); !strings.Contains(got, "no changes") {
		t.Fatalf("permission detail=%q, want no changes", got)
	}
}

func TestUpdateTodoForSessionIncludesSessionIdentity(t *testing.T) {
	store, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewUpdateTodoForSession(store, "session-123")
	call, err := tool.NewCall("update-session", "update_todo", todoPatchArgs(0, map[string]any{
		"op": "add", "id": "a", "text": "one", "status": "pending",
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.StructuredOutput, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["session_id"] != "session-123" {
		t.Fatalf("session_id=%v", payload["session_id"])
	}
}
