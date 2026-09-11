package todotool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
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
	h := newUpdateTodo(store)
	call, _ := tool.NewCall("todo-1", "todo", todoPatchArgs(0,
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
	h := newUpdateTodo(store)
	call, _ := tool.NewCall("todo-1", "todo", todoPatchArgs(0,
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
	h := newUpdateTodo(store)
	call, _ := tool.NewCall("todo-diff", "todo", todoPatchArgs(0,
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

func TestTodoChangesReopenDoesNotAlsoCountStarted(t *testing.T) {
	before := []tododomain.Item{{ID: "a", Text: "a", Status: tododomain.StatusCompleted}}
	after := []tododomain.Item{{ID: "a", Text: "a", Status: tododomain.StatusInProgress}}
	changes := todoChanges(before, after)
	if changes.Reopened != 1 || changes.Started != 0 || changes.Completed != 0 {
		t.Fatalf("changes=%+v, want reopened only", changes)
	}
}

func TestUpdateTodoReportsTextUpdates(t *testing.T) {
	store, _ := tododomain.NewStore([]tododomain.Item{{ID: "a", Text: "old", Status: tododomain.StatusPending}})
	h := newUpdateTodo(store)
	call, _ := tool.NewCall("todo-text", "todo", todoPatchArgs(0, map[string]any{"op": "set_text", "id": "a", "text": "new"}))
	result, err := h.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "1 updated") {
		t.Fatalf("output=%q, want text update summary", result.Output)
	}
	if !strings.Contains(string(result.StructuredOutput), `"updated":1`) {
		t.Fatalf("structured output=%s", result.StructuredOutput)
	}
}

func TestTodoCapabilityDefinitionUsesClosedTypedRootSchema(t *testing.T) {
	def := NewTodo(nil).Definition()
	if def.InputSchema["type"] != "object" {
		t.Fatalf("input schema type = %#v, want object", def.InputSchema["type"])
	}
	if def.InputSchema["additionalProperties"] != false {
		t.Fatalf("additionalProperties = %#v, want false", def.InputSchema["additionalProperties"])
	}
	props := def.InputSchema["properties"].(map[string]any)
	if got := props["expected_revision"].(map[string]any)["type"]; got != "integer" {
		t.Fatalf("expected_revision type = %#v, want integer", got)
	}
	if got := props["operations"].(map[string]any)["type"]; got != "array" {
		t.Fatalf("operations type = %#v, want array", got)
	}
	branches, ok := def.InputSchema["oneOf"].([]any)
	if !ok || len(branches) != 2 {
		t.Fatalf("oneOf = %#v, want get/update branches", def.InputSchema["oneOf"])
	}
	if err := def.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateTodoDefinitionUsesPatchSchema(t *testing.T) {
	def := newUpdateTodo(nil).Definition()
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
	branches, ok := items["oneOf"].([]any)
	if !ok || len(branches) != 4 {
		t.Fatalf("operation oneOf = %#v, want 4 branches", items["oneOf"])
	}
	wantRequired := map[string][]string{
		"add":        {"op", "id", "text", "status"},
		"set_status": {"op", "id", "status"},
		"set_text":   {"op", "id", "text"},
		"remove":     {"op", "id"},
	}
	for _, raw := range branches {
		branch := raw.(map[string]any)
		branchProps := branch["properties"].(map[string]any)
		op := branchProps["op"].(map[string]any)["const"].(string)
		required := branch["required"].([]any)
		if len(required) != len(wantRequired[op]) {
			t.Fatalf("%s required = %#v", op, required)
		}
	}
	if err := def.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateTodoRejectsStaleRevisionEvenWhenPatchWouldApply(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := newUpdateTodo(store)
	first, _ := tool.NewCall("todo-first", "todo", todoPatchArgs(0, map[string]any{"op": "add", "id": "a", "text": "one", "status": "pending"}))
	if _, err := h.Execute(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	stale, _ := tool.NewCall("todo-stale", "todo", todoPatchArgs(0, map[string]any{"op": "add", "id": "b", "text": "two", "status": "pending"}))
	_, err := h.Execute(context.Background(), stale)
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeConflict {
		t.Fatalf("error = %v, want conflict", err)
	}
	got := store.Snapshot()
	if got.Revision != 1 || len(got.Items) != 1 || got.Items[0].ID != "a" {
		t.Fatalf("stale patch mutated snapshot: %#v", got)
	}
}

func TestUpdateTodoStaleRevisionReturnsStructuredRefreshRecovery(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := newUpdateTodo(store)
	first, _ := tool.NewCall("todo-first", "todo", todoPatchArgs(0, map[string]any{"op": "add", "id": "a", "text": "one", "status": "pending"}))
	if _, err := h.Execute(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	stale, _ := tool.NewCall("todo-stale", "todo", todoPatchArgs(0, map[string]any{"op": "add", "id": "a", "text": "two", "status": "pending"}))
	_, err := h.Execute(context.Background(), stale)
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeConflict {
		t.Fatalf("error = %v, want conflict", err)
	}
	if toolErr.Recovery == nil || toolErr.Recovery.Action != tool.RecoveryRefreshResource || toolErr.Recovery.Tool != "todo" || string(toolErr.Recovery.Arguments) != `{"action":"get"}` {
		t.Fatalf("recovery = %#v", toolErr.Recovery)
	}
}

func TestUpdateTodoRequiresExpectedRevisionAndOperations(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := newUpdateTodo(store)
	for _, args := range []json.RawMessage{
		json.RawMessage(`{"operations":[{"op":"add","id":"a","text":"a","status":"pending"}]}`),
		json.RawMessage(`{"expected_revision":0,"operations":[]}`),
	} {
		call, _ := tool.NewCall("todo-invalid", "todo", args)
		_, err := h.Execute(context.Background(), call)
		var toolErr *tool.ToolError
		if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeInvalidArguments {
			t.Fatalf("error=%v, want invalid arguments", err)
		}
	}
}

func TestUpdateTodoRejectsLegacySnapshotAndUnknownFields(t *testing.T) {
	store, _ := tododomain.NewStore(nil)
	h := newUpdateTodo(store)
	for _, args := range []json.RawMessage{
		json.RawMessage(`{"expected_revision":0,"items":[]}`),
		json.RawMessage(`{"expected_revision":0,"operations":[{"op":"add","id":"a","text":"a","status":"pending","banana":true}]}`),
	} {
		call, _ := tool.NewCall("todo-legacy", "todo", args)
		if _, err := h.Execute(context.Background(), call); err == nil {
			t.Fatalf("arguments %s unexpectedly accepted", args)
		}
	}
}

func TestUpdateTodoPermissionDetailDoesNotClaimDurableState(t *testing.T) {
	store, _ := tododomain.NewStore([]tododomain.Item{{ID: "a", Text: "a", Status: tododomain.StatusPending}})
	h := newUpdateTodo(store).(updateTodoHandler)
	args := todoPatchArgs(7,
		map[string]any{"op": "set_status", "id": "a", "status": "completed"},
		map[string]any{"op": "add", "id": "b", "text": "b", "status": "pending"},
	)
	if got := h.PermissionDetail(args); got != "2 task operations · expected revision 7" {
		t.Fatalf("permission detail=%q", got)
	}
}

func TestUpdateTodoForSessionIncludesSessionIdentity(t *testing.T) {
	store, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := newUpdateTodoForSession(store, "session-123")
	call, err := tool.NewCall("update-session", "todo", todoPatchArgs(0, map[string]any{
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

func TestUpdateTodoInvalidPatchPublishesActionableDiagnostic(t *testing.T) {
	store, err := tododomain.NewStore([]tododomain.Item{{ID: "a", Text: "keep", Status: tododomain.StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	h := newUpdateTodo(store)
	call, _ := tool.NewCall("todo-invalid-patch", "todo", todoPatchArgs(0, map[string]any{
		"op": "set_status", "id": "a", "status": "completed", "text": "not allowed",
	}))
	_, err = h.Execute(context.Background(), call)
	var toolErr *tool.ToolError
	if !errors.As(err, &toolErr) || toolErr.Code != tool.ErrorCodeInvalidArguments {
		t.Fatalf("error=%v, want invalid arguments", err)
	}
	if !strings.Contains(toolErr.Diagnostic, "set_status does not accept text") {
		t.Fatalf("diagnostic=%q", toolErr.Diagnostic)
	}
	failure := tool.FailureFromError(err)
	if failure == nil || !strings.Contains(failure.Diagnostic, "set_status does not accept text") {
		t.Fatalf("failure=%#v", failure)
	}
}
