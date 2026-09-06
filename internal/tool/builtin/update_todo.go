package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
	if h.store == nil {
		return fmt.Sprintf("%d tasks", len(input.Items))
	}
	current := h.store.Snapshot()
	if input.ExpectedRevision == nil || *input.ExpectedRevision != current.Revision {
		return fmt.Sprintf("stale task plan · %d tasks", len(input.Items))
	}
	changes := todoChanges(current.Items, input.Items)
	if detail := summarizeTodoChanges(changes); detail != "" {
		return detail
	}
	return fmt.Sprintf("%d tasks · no changes", len(input.Items))
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
	before := h.store.Snapshot()
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
	changes := todoChanges(before.Items, snapshot.Items)
	payload, err := json.Marshal(map[string]any{
		"revision":    snapshot.Revision,
		"total":       len(snapshot.Items),
		"pending":     counts[tododomain.StatusPending],
		"in_progress": counts[tododomain.StatusInProgress],
		"completed":   counts[tododomain.StatusCompleted],
		"changes":     changes,
	})
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "encode todo result", err)
	}
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: string(payload)}, nil
}

type todoChangeSummary struct {
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Started   int `json:"started"`
	Completed int `json:"completed"`
	Reopened  int `json:"reopened"`
}

func todoChanges(before, after []tododomain.Item) todoChangeSummary {
	old := make(map[string]tododomain.Item, len(before))
	for _, item := range before {
		old[item.ID] = item
	}
	next := make(map[string]tododomain.Item, len(after))
	var out todoChangeSummary
	for _, item := range after {
		next[item.ID] = item
		prev, ok := old[item.ID]
		if !ok {
			out.Added++
			continue
		}
		if prev.Status != tododomain.StatusInProgress && item.Status == tododomain.StatusInProgress {
			out.Started++
		}
		if prev.Status != tododomain.StatusCompleted && item.Status == tododomain.StatusCompleted {
			out.Completed++
		}
		if prev.Status == tododomain.StatusCompleted && item.Status != tododomain.StatusCompleted {
			out.Reopened++
		}
	}
	for _, item := range before {
		if _, ok := next[item.ID]; !ok {
			out.Removed++
		}
	}
	return out
}

func summarizeTodoChanges(changes todoChangeSummary) string {
	parts := make([]string, 0, 5)
	appendChange := func(count int, label string) {
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, label))
		}
	}
	appendChange(changes.Completed, "completed")
	appendChange(changes.Started, "started")
	appendChange(changes.Reopened, "reopened")
	appendChange(changes.Added, "added")
	appendChange(changes.Removed, "removed")
	return strings.Join(parts, " · ")
}
