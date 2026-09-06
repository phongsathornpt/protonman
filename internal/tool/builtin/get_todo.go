package builtin

import (
	"context"
	"encoding/json"

	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
)

type getTodoHandler struct{ store tododomain.Repository }

func NewGetTodo(store tododomain.Repository) tool.Handler { return getTodoHandler{store: store} }

func (getTodoHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        "get_todo",
		Description: "Read the current parent-owned task snapshot and revision before updating it.",
		Kind:        tool.KindTask,
		Mutability:  tool.MutabilityReadOnly,
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}
}

func (h getTodoHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	if h.store == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "todo store is not configured")
	}
	payload, err := json.Marshal(h.store.Snapshot())
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "encode todo snapshot", err)
	}
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: string(payload)}, nil
}
