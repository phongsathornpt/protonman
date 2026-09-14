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
	return todoHandler{get: newGetTodo(store), update: newUpdateTodo(store)}
}

// NewTodoForSession exposes session-scoped task planning as one compact capability.
func NewTodoForSession(store tododomain.Repository, sessionID string) tool.Handler {
	return todoHandler{get: newGetTodoForSession(store, sessionID), update: newUpdateTodoForSession(store, sessionID)}
}

func (h todoHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        tool.NameTodo,
		Description: "Task-plan capability. Use action=get when the current revision is unknown, or action=update to atomically patch tasks using the latest known expectedRevision and operations. A successful update returns the next revision.",
		Kind:        tool.KindTask,
		Mutability:  tool.MutabilityMutating,
		Safety:      tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone},
		Semantics:   h.callSemantics,
		InputAliases: map[string][]string{
			"expectedRevision": {"expected_revision"},
			"sessionId":        {"session_id"},
		},
		InputSchema: todoCapabilityInputSchema(),
		OutputSchema: map[string]any{
			"oneOf": []any{todoSnapshotSchema(), todoUpdateOutputSchema()},
		},
	}
}

func todoCapabilityInputSchema() map[string]any {
	update := todoUpdateInputSchema()
	updateProps, _ := update["properties"].(map[string]any)
	props := map[string]any{"action": map[string]any{
		"type":        "string",
		"enum":        []any{"get", "update"},
		"description": "Use get when the current revision is unknown; use update with the latest integer revision returned by get or a prior successful update and an operations JSON array.",
	}}
	for name, schema := range updateProps {
		props[name] = schema
	}
	// sessionId is echoed from the session-bound get snapshot often enough that
	// rejecting it would fail a well-intentioned call; it is informational here
	// because the runtime always owns the real session binding.
	props["sessionId"] = map[string]any{"type": "string", "description": "Optional session identity echoed from a todo action=get snapshot. Ignored; the runtime owns session binding."}
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             []any{"action"},
		"additionalProperties": false,
		"allOf": []any{map[string]any{
			// Argument normalization canonicalizes case variants of the action
			// before validation, so the condition only needs the canonical form.
			"if": map[string]any{
				"properties": map[string]any{"action": map[string]any{"const": "update"}},
				"required":   []any{"action"},
			},
			"then": map[string]any{"required": []any{"expectedRevision", "operations"}},
		}},
	}
}

// NormalizeArguments implements tool.ArgumentNormalizer so wire variants such as
// a quoted revision are canonicalized before schema validation rejects them.
func (h todoHandler) NormalizeArguments(arguments json.RawMessage) json.RawMessage {
	return taskArgumentNormalizer{}.NormalizeArguments(arguments)
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

// branchAction returns the normalized action kind. ok is false when the
// arguments are not a JSON object or carry no readable action, which keeps
// permission detail from acting on malformed input.
func branchAction(arguments json.RawMessage) (string, bool) {
	object := map[string]json.RawMessage{}
	if json.Unmarshal(arguments, &object) != nil {
		return "", false
	}
	raw, ok := object["action"]
	if !ok {
		return "", false
	}
	var action string
	if json.Unmarshal(raw, &action) != nil {
		return "", false
	}
	return strings.ToLower(strings.TrimSpace(action)), true
}

func (h todoHandler) PermissionDetail(arguments json.RawMessage) string {
	action, ok := branchAction(arguments)
	if !ok {
		// Without a readable action the call will fail validation; describe the
		// capability so the prompt still explains what the model attempted.
		return "task plan"
	}
	child, ok := h.child(action)
	if !ok {
		return "task plan"
	}
	childArgs, _, err := h.resolve(arguments)
	if err != nil {
		return "task plan"
	}
	if provider, ok := child.(tool.DetailProvider); ok {
		if detail := strings.TrimSpace(provider.PermissionDetail(childArgs)); detail != "" {
			return detail
		}
	}
	return "task plan"
}

func (h todoHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	childArgs, child, err := h.resolve(call.Arguments)
	if err != nil {
		return tool.Result{}, err
	}
	childCall := call
	childCall.Arguments = tool.NormalizeArguments(child.Definition(), childArgs)
	result, err := child.Execute(ctx, childCall)
	result.ToolName = call.Name
	return result, err
}

// resolve selects the handler for the requested action and narrows the arguments
// to the payload that action owns.
func (h todoHandler) resolve(arguments json.RawMessage) (json.RawMessage, tool.Handler, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &object); err != nil {
		return nil, nil, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode todo arguments", err)
	}
	var action string
	if raw, ok := object["action"]; ok {
		_ = json.Unmarshal(raw, &action)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	child, ok := h.child(action)
	if !ok {
		return nil, nil, tool.NewToolError(tool.ErrorCodeInvalidArguments, "todo action must be get or update")
	}
	delete(object, "action")
	// sessionId is accepted as an informational echo of a get snapshot and is
	// never forwarded to the patch handler, which does not own session identity.
	delete(object, "session_id")
	delete(object, "sessionId")
	if action == "get" {
		// operations means the model intended to patch. Failing loudly is safer
		// than silently discarding an intended mutation.
		if _, ok := object["operations"]; ok {
			return nil, nil, tool.NewToolError(tool.ErrorCodeInvalidArguments, "todo action=get does not accept operations; use action=update to patch the task plan")
		}
		// expectedRevision is a harmless echo of the snapshot the caller just
		// read, so it is tolerated rather than reported as misuse.
		delete(object, "expected_revision")
		delete(object, "expectedRevision")
		if len(object) != 0 {
			return nil, nil, tool.NewToolError(tool.ErrorCodeInvalidArguments, "todo action=get does not accept update arguments")
		}
	}
	childArgs, err := json.Marshal(object)
	if err != nil {
		return nil, nil, fmt.Errorf("encode todo %s arguments: %w", action, err)
	}
	return childArgs, child, nil
}
