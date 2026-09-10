package todotool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

type updateTodoHandler struct {
	store     tododomain.Repository
	sessionID string
}

type updateTodoInput struct {
	ExpectedRevision *uint64                `json:"expected_revision"`
	Operations       []tododomain.Operation `json:"operations"`
}

func newUpdateTodo(store tododomain.Repository) tool.Handler {
	return updateTodoHandler{store: store}
}

func newUpdateTodoForSession(store tododomain.Repository, sessionID string) tool.Handler {
	return updateTodoHandler{store: store, sessionID: sessionID}
}

func (updateTodoHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:         tool.NameTodo,
		Description:  "Patch the parent-owned task plan atomically using explicit add, set_status, set_text, or remove operations.",
		Kind:         tool.KindTask,
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
	if input.ExpectedRevision == nil {
		return fmt.Sprintf("%d task operations", len(input.Operations))
	}
	return fmt.Sprintf("%d task operations · expected revision %d", len(input.Operations), *input.ExpectedRevision)
}

func (h updateTodoHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.store == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "todo store is not configured")
	}
	input, err := decodeUpdateTodoInput(call.Arguments)
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode todo update arguments", err)
	}
	if input.ExpectedRevision == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "expected_revision is required; call todo with action=get first")
	}
	if len(input.Operations) == 0 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "operations must contain at least one explicit todo patch")
	}
	if len(input.Operations) > 256 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "operations exceed the 256-operation limit")
	}
	expected := *input.ExpectedRevision
	var before, snapshot tododomain.Snapshot
	usedAtomicPatch := false
	if patcher, ok := h.store.(tododomain.PatchRepository); ok {
		usedAtomicPatch = true
		before, snapshot, err = patcher.CompareAndPatch(ctx, expected, input.Operations)
	} else {
		before = h.store.Snapshot()
		if reloader, ok := h.store.(tododomain.ReloadableRepository); ok {
			before, err = reloader.Reload(ctx)
			if err != nil {
				return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "reload todo snapshot", err)
			}
		}
		if before.Revision != expected {
			err = tododomain.ErrRevisionConflict
		} else {
			var next []tododomain.Item
			next, err = tododomain.ApplyPatch(before.Items, input.Operations)
			if err == nil {
				snapshot, err = h.store.CompareAndReplace(ctx, expected, next)
			}
		}
	}
	if err != nil {
		if errors.Is(err, tododomain.ErrRevisionConflict) {
			return tool.Result{}, todoConflictError(err)
		}
		if usedAtomicPatch && before.Revision == expected {
			if _, patchErr := tododomain.ApplyPatch(before.Items, input.Operations); patchErr != nil {
				return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "apply todo patch", patchErr)
			}
		}
		return tool.Result{}, err
	}
	return encodeTodoUpdateResult(call, before.Items, snapshot, h.sessionID)
}

func todoConflictError(cause error) error {
	return tool.WrapToolError(tool.ErrorCodeConflict, "todo snapshot changed; refresh tasks before applying this patch", cause).WithRecovery(tool.Recovery{
		Action: tool.RecoveryRefreshResource, Tool: "todo", Arguments: json.RawMessage(`{"action":"get"}`),
	})
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

func encodeTodoUpdateResult(call tool.Call, before []tododomain.Item, snapshot tododomain.Snapshot, sessionID string) (tool.Result, error) {
	counts := map[tododomain.Status]int{}
	for _, item := range snapshot.Items {
		counts[item.Status]++
	}
	changes := todoChanges(before, snapshot.Items)
	payloadValue := map[string]any{
		"revision": snapshot.Revision, "total": len(snapshot.Items),
		"pending": counts[tododomain.StatusPending], "in_progress": counts[tododomain.StatusInProgress],
		"completed": counts[tododomain.StatusCompleted], "changes": changes,
	}
	if sessionID != "" {
		payloadValue["session_id"] = sessionID
	}
	payload, err := json.Marshal(payloadValue)
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
	Updated   int `json:"updated"`
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
		if prev.Text != item.Text {
			out.Updated++
		}
		switch {
		case prev.Status == tododomain.StatusCompleted && item.Status != tododomain.StatusCompleted:
			out.Reopened++
		case prev.Status != tododomain.StatusCompleted && item.Status == tododomain.StatusCompleted:
			out.Completed++
		case prev.Status != tododomain.StatusInProgress && item.Status == tododomain.StatusInProgress:
			out.Started++
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
	appendChange(changes.Updated, "updated")
	appendChange(changes.Added, "added")
	appendChange(changes.Removed, "removed")
	return strings.Join(parts, " · ")
}
