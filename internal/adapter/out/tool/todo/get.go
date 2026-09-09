package todotool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

type getTodoHandler struct {
	store     tododomain.Repository
	sessionID string
}

func NewGetTodo(store tododomain.Repository) tool.Handler { return getTodoHandler{store: store} }

func NewGetTodoForSession(store tododomain.Repository, sessionID string) tool.Handler {
	return getTodoHandler{store: store, sessionID: sessionID}
}

func (getTodoHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:         tool.NameTodo,
		Description:  "Read the current parent-owned task snapshot and revision.",
		Kind:         tool.KindTask,
		Mutability:   tool.MutabilityReadOnly,
		Safety:       tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone},
		InputSchema:  tool.NoArgumentsSchema(),
		OutputSchema: todoSnapshotSchema(),
	}
}

func (h getTodoHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if err := decodeGetTodoInput(call.Arguments); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode todo get arguments", err)
	}
	if h.store == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "todo store is not configured")
	}
	snapshot := h.store.Snapshot()
	if reloader, ok := h.store.(tododomain.ReloadableRepository); ok {
		var err error
		snapshot, err = reloader.Reload(ctx)
		if err != nil {
			return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "reload todo snapshot", err)
		}
	}
	var payload []byte
	var err error
	if h.sessionID == "" {
		payload, err = json.Marshal(snapshot)
	} else {
		payload, err = json.Marshal(map[string]any{"session_id": h.sessionID, "revision": snapshot.Revision, "items": snapshot.Items})
	}
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "encode todo snapshot", err)
	}
	return tool.Result{
		CallID:           call.ID,
		ToolName:         call.Name,
		Output:           fmt.Sprintf("task snapshot revision %d · %d tasks", snapshot.Revision, len(snapshot.Items)),
		StructuredOutput: payload,
	}, nil
}

func decodeGetTodoInput(arguments json.RawMessage) error {
	arguments = tool.NormalizeArguments(getTodoHandler{}.Definition(), arguments)
	decoder := json.NewDecoder(bytes.NewReader(arguments))
	decoder.DisallowUnknownFields()
	var input struct{}
	if err := decoder.Decode(&input); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
