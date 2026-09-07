package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	tododomain "github.com/projectTHORN/proton/internal/todo"
	"github.com/projectTHORN/proton/internal/tool"
)

type getTodoHandler struct{ store tododomain.Repository }

func NewGetTodo(store tododomain.Repository) tool.Handler { return getTodoHandler{store: store} }

func (getTodoHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:         "get_todo",
		Description:  "Read the current parent-owned task snapshot and revision before applying update_todo patch operations. This tool takes no arguments; call it with an empty JSON object {}.",
		Kind:         tool.KindForName("get_todo"),
		Mutability:   tool.MutabilityReadOnly,
		Safety:       tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone},
		InputSchema:  tool.NoArgumentsSchema(),
		OutputSchema: todoSnapshotSchema(),
	}
}

func (h getTodoHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	if err := decodeGetTodoInput(call.Arguments); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode get_todo arguments", err)
	}
	if h.store == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "todo store is not configured")
	}
	snapshot := h.store.Snapshot()
	payload, err := json.Marshal(snapshot)
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
