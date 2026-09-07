package builtin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
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
	for _, raw := range []string{`{}`, ``, `   `, `null`} {
		call := tool.Call{ID: "todo-get", Name: "get_todo", Arguments: json.RawMessage(raw)}
		if _, err := h.Execute(context.Background(), call); err != nil {
			t.Fatalf("get_todo(%q) error = %v", raw, err)
		}
	}
	for _, raw := range []string{`[]`, `""`, `{"foo":1}`, `{} {}`} {
		call := tool.Call{ID: "todo-get", Name: "get_todo", Arguments: json.RawMessage(raw)}
		if _, err := h.Execute(context.Background(), call); err == nil {
			t.Fatalf("get_todo(%q) error = nil, want invalid arguments", raw)
		}
	}
}
