package builtin

import (
	"context"
	"strings"
	"testing"

	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
)

func TestGetTodoReturnsSnapshotRevision(t *testing.T) {
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
	if !strings.Contains(res.Output, `"revision":0`) || !strings.Contains(res.Output, `"id":"a"`) {
		t.Fatalf("output=%s", res.Output)
	}
	def := h.Definition()
	if def.Kind != tool.KindTask || def.Mutability != tool.MutabilityReadOnly {
		t.Fatalf("definition=%#v", def)
	}
}
