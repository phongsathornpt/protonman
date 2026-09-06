package builtin

import (
	"context"
	"encoding/json"
	"fmt"

	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
)

type updateTodoHandler struct {
	store tododomain.Repository
}

type updateTodoInput struct {
	Items []tododomain.Item `json:"items"`
}

func NewUpdateTodo(store tododomain.Repository) tool.Handler {
	return updateTodoHandler{store: store}
}

func (updateTodoHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        "update_todo",
		Description: "Replace the structured project task plan atomically, preserving stable task IDs and statuses.",
		Kind:        tool.KindTask,
		Mutability:  tool.MutabilityMutating,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"items": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":     map[string]any{"type": "string"},
							"text":   map[string]any{"type": "string"},
							"status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
						},
						"required": []string{"id", "text", "status"},
					},
				},
			},
			"required": []string{"items"},
		},
	}
}

func (h updateTodoHandler) PermissionDetail(arguments json.RawMessage) string {
	var input updateTodoInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "task plan"
	}
	return fmt.Sprintf("%d tasks", len(input.Items))
}

func (h updateTodoHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.store == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "todo store is not configured")
	}
	var input updateTodoInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode update_todo arguments", err)
	}
	if err := tododomain.ValidateItems(input.Items); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "validate todo items", err)
	}
	snapshot, err := h.store.Replace(ctx, input.Items)
	if err != nil {
		return tool.Result{}, err
	}
	counts := map[tododomain.Status]int{}
	for _, item := range snapshot.Items {
		counts[item.Status]++
	}
	payload, err := json.Marshal(map[string]any{
		"revision":    snapshot.Revision,
		"total":       len(snapshot.Items),
		"pending":     counts[tododomain.StatusPending],
		"in_progress": counts[tododomain.StatusInProgress],
		"completed":   counts[tododomain.StatusCompleted],
	})
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "encode todo result", err)
	}
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: string(payload)}, nil
}
