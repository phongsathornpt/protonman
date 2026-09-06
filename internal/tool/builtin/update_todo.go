package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
)

type updateTodoHandler struct {
	store tododomain.Repository
}

type updateTodoInput struct {
	ExpectedRevision *uint64           `json:"expected_revision"`
	Items            []tododomain.Item `json:"items"`
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
				"expected_revision": map[string]any{
					"type": "integer", "minimum": 0,
					"description": "Revision from the latest task snapshot; stale revisions are rejected.",
				},
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
			"required": []string{"expected_revision", "items"},
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
	if input.ExpectedRevision == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "expected_revision is required; call get_todo first")
	}
	if err := tododomain.ValidateItems(input.Items); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "validate todo items", err)
	}
	snapshot, err := h.store.CompareAndReplace(ctx, *input.ExpectedRevision, input.Items)
	if err != nil {
		if errors.Is(err, tododomain.ErrRevisionConflict) {
			return tool.Result{}, tool.WrapToolError(tool.ErrorCodeConflict, "todo snapshot is stale; refresh tasks and retry", err)
		}
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
