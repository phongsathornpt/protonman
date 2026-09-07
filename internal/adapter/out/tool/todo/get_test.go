package todotool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/core/tool"
	tododomain "github.com/projectTHORN/proton/internal/feature/todo"
)

func TestGetTodoReturnsStructuredSnapshotRevision(t *testing.T) {
	store, err := tododomain.NewStore([]tododomain.Item{{ID: "a", Text: "inspect", Status: tododomain.StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	h := NewGetTodo(store)
	call, _ := tool.NewCall("todo-get", "get_todo", []byte(`{}`))
	res, err := h.Execute(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "revision 0") || !strings.Contains(res.Output, "1 tasks") {
		t.Fatalf("output=%s", res.Output)
	}
	var snapshot tododomain.Snapshot
	if err := json.Unmarshal(res.StructuredOutput, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Revision != 0 || len(snapshot.Items) != 1 || snapshot.Items[0].ID != "a" {
		t.Fatalf("structured snapshot=%#v", snapshot)
	}
	def := h.Definition()
	if def.Kind != tool.KindTask || def.Mutability != tool.MutabilityReadOnly || len(def.OutputSchema) == 0 {
		t.Fatalf("definition=%#v", def)
	}
}

func TestGetTodoArgumentContract(t *testing.T) {
	store, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	h := NewGetTodo(store)
	for _, raw := range []string{`{}`, ``, `   `, `null`, `{"reason":"checking tasks"}`, `{"foo":1}`} {
		call := tool.Call{ID: "todo-get", Name: "get_todo", Arguments: json.RawMessage(raw)}
		if _, err := h.Execute(context.Background(), call); err != nil {
			t.Fatalf("get_todo(%q) error = %v", raw, err)
		}
	}
	for _, raw := range []string{`[]`, `""`, `1`, `true`, `{} {`} {
		call := tool.Call{ID: "todo-get", Name: "get_todo", Arguments: json.RawMessage(raw)}
		if _, err := h.Execute(context.Background(), call); err == nil {
			t.Fatalf("get_todo(%q) error = nil, want invalid arguments", raw)
		}
	}
}

func TestGetTodoForSessionIncludesSessionIdentity(t *testing.T) {
	store, err := tododomain.NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewGetTodoForSession(store, "session-123")
	call, err := tool.NewCall("get-session", "get_todo", json.RawMessage(`{}`))
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
