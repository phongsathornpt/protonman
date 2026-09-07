package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
)

type updateTodoHandler struct {
	store tododomain.Repository
}

type updateTodoInput struct {
	ExpectedRevision *uint64                `json:"expected_revision"`
	Operations       []tododomain.Operation `json:"operations"`
}

func NewUpdateTodo(store tododomain.Repository) tool.Handler {
	return updateTodoHandler{store: store}
}

func (updateTodoHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:         "update_todo",
		Description:  "Patch the parent-owned task plan atomically using explicit add, set_status, set_text, or remove operations.",
		Kind:         tool.KindForName("update_todo"),
		Mutability:   tool.MutabilityMutating,
		Safety:       tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone},
		InputSchema:  todoUpdateInputSchema(),
		OutputSchema: todoUpdateOutputSchema(),
	}
}

func (h updateTodoHandler) PermissionDetail(arguments json.RawMessage) string {
	var input updateTodoInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "task patch"
	}
	if h.store == nil {
		return fmt.Sprintf("%d task operations", len(input.Operations))
	}
	current := h.store.Snapshot()
	if input.ExpectedRevision == nil || *input.ExpectedRevision != current.Revision {
		return fmt.Sprintf("stale task patch · %d operations", len(input.Operations))
	}
	next, err := tododomain.ApplyPatch(current.Items, input.Operations)
	if err != nil {
		return fmt.Sprintf("invalid task patch · %d operations", len(input.Operations))
	}
	changes := todoChanges(current.Items, next)
	if detail := summarizeTodoChanges(changes); detail != "" {
		return detail
	}
	return fmt.Sprintf("%d task operations · no changes", len(input.Operations))
}

func (h updateTodoHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.store == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "todo store is not configured")
	}
	input, err := decodeUpdateTodoInput(call.Arguments)
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode update_todo arguments", err)
	}
	if input.ExpectedRevision == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "expected_revision is required; call get_todo first")
	}
	if len(input.Operations) == 0 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "operations must contain at least one explicit todo patch")
	}
	if len(input.Operations) > 256 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "operations exceed the 256-operation limit")
	}
	before := h.store.Snapshot()
	if before.Revision != *input.ExpectedRevision {
		err := fmt.Errorf("%w: expected %d, current %d", tododomain.ErrRevisionConflict, *input.ExpectedRevision, before.Revision)
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeConflict, "todo snapshot is stale; refresh tasks and retry", err)
	}
	next, err := tododomain.ApplyPatch(before.Items, input.Operations)
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "apply todo patch", err)
	}
	snapshot, err := h.store.CompareAndReplace(ctx, *input.ExpectedRevision, next)
	if err != nil {
		if errors.Is(err, tododomain.ErrRevisionConflict) {
			return tool.Result{}, tool.WrapToolError(tool.ErrorCodeConflict, "todo snapshot is stale; refresh tasks and retry", err)
		}
		return tool.Result{}, err
	}
	return encodeTodoUpdateResult(call, before.Items, snapshot)
}

func decodeUpdateTodoInput(arguments json.RawMessage) (updateTodoInput, error) {
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	var input updateTodoInput
	if err := decoder.Decode(&input); err != nil {
		return updateTodoInput{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return updateTodoInput{}, errors.New("multiple JSON values are not allowed")
		}
		return updateTodoInput{}, err
	}
	return input, nil
}

func encodeTodoUpdateResult(call tool.Call, before []tododomain.Item, snapshot tododomain.Snapshot) (tool.Result, error) {
	counts := map[tododomain.Status]int{}
	for _, item := range snapshot.Items {
		counts[item.Status]++
	}
	changes := todoChanges(before, snapshot.Items)
	payload, err := json.Marshal(map[string]any{
		"revision": snapshot.Revision, "total": len(snapshot.Items),
		"pending": counts[tododomain.StatusPending], "in_progress": counts[tododomain.StatusInProgress],
		"completed": counts[tododomain.StatusCompleted], "changes": changes,
	})
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "encode todo result", err)
	}
	detail := summarizeTodoChanges(changes)
	if detail == "" {
		detail = "no changes"
	}
	return tool.Result{
		CallID: call.ID, ToolName: call.Name,
		Output:           fmt.Sprintf("task plan revision %d · %d tasks · %s", snapshot.Revision, len(snapshot.Items), detail),
		StructuredOutput: payload,
	}, nil
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
