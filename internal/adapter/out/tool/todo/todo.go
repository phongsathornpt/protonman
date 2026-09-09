package todotool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

type todoHandler struct {
	get    tool.Handler
	update tool.Handler
}

// NewTodo exposes task planning as one compact capability.
func NewTodo(store tododomain.Repository) tool.Handler {
	return todoHandler{get: NewGetTodo(store), update: NewUpdateTodo(store)}
}

// NewTodoForSession exposes session-scoped task planning as one compact capability.
func NewTodoForSession(store tododomain.Repository, sessionID string) tool.Handler {
	return todoHandler{get: NewGetTodoForSession(store, sessionID), update: NewUpdateTodoForSession(store, sessionID)}
}

func (h todoHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        tool.NameTodo,
		Description: "Task-plan capability. Use action=get to read the current revision and tasks, or action=update to atomically patch tasks using expected_revision and operations.",
		Kind:        tool.KindTask,
		Mutability:  tool.MutabilityMutating,
		Safety:      tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone},
		Semantics:   h.callSemantics,
		InputSchema: todoCapabilityInputSchema(),
		OutputSchema: map[string]any{
			"oneOf": []any{todoSnapshotSchema(), todoUpdateOutputSchema()},
		},
	}
}

func todoCapabilityInputSchema() map[string]any {
	update := todoUpdateInputSchema()
	props, _ := update["properties"].(map[string]any)
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":            map[string]any{"type": "string", "enum": []string{"get", "update"}, "description": "Task-plan operation to perform"},
			"expected_revision": props["expected_revision"],
			"operations":        props["operations"],
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	}
}

func (h todoHandler) child(action string) (tool.Handler, bool) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "get":
		return h.get, true
	case "update":
		return h.update, true
	default:
		return nil, false
	}
}

func (h todoHandler) callSemantics(arguments json.RawMessage) tool.CallSemantics {
	fallback := tool.CallSemantics{Mutability: tool.MutabilityMutating, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}}
	var in struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(arguments, &in) != nil {
		return fallback
	}
	child, ok := h.child(in.Action)
	if !ok {
		return fallback
	}
	return tool.StaticCallSemantics(child.Definition())
}

func (h todoHandler) PermissionDetail(arguments json.RawMessage) string {
	_, childArgs, child, err := h.resolve(arguments)
	if err != nil {
		return ""
	}
	if provider, ok := child.(tool.DetailProvider); ok {
		return provider.PermissionDetail(childArgs)
	}
	return ""
}

func (h todoHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	_, childArgs, child, err := h.resolve(call.Arguments)
	if err != nil {
		return tool.Result{}, err
	}
	childCall := call
	childCall.Arguments = tool.NormalizeArguments(child.Definition(), childArgs)
	result, err := child.Execute(ctx, childCall)
	result.ToolName = call.Name
	return result, err
}

func (h todoHandler) resolve(arguments json.RawMessage) (string, json.RawMessage, tool.Handler, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &object); err != nil {
		return "", nil, nil, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode todo arguments", err)
	}
	var action string
	if raw, ok := object["action"]; ok {
		_ = json.Unmarshal(raw, &action)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	child, ok := h.child(action)
	if !ok {
		return "", nil, nil, tool.NewToolError(tool.ErrorCodeInvalidArguments, "todo action must be get or update")
	}
	delete(object, "action")
	if action == "get" {
		if len(object) != 0 {
			return "", nil, nil, tool.NewToolError(tool.ErrorCodeInvalidArguments, "todo action=get does not accept update arguments")
		}
	}
	childArgs, err := json.Marshal(object)
	if err != nil {
		return "", nil, nil, fmt.Errorf("encode todo %s arguments: %w", action, err)
	}
	return action, childArgs, child, nil
}
