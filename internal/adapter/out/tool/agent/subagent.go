package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type subagentAction string

const (
	subagentActionSpawn  subagentAction = "spawn"
	subagentActionWait   subagentAction = "wait"
	subagentActionGet    subagentAction = "get"
	subagentActionList   subagentAction = "list"
	subagentActionCancel subagentAction = "cancel"
	subagentActionResume subagentAction = "resume"
)

type subagentHandler struct {
	spawn  tool.Handler
	wait   tool.Handler
	get    tool.Handler
	list   tool.Handler
	cancel tool.Handler
	resume tool.Handler
}

// NewSubagent exposes subagent orchestration as one capability with an action discriminator.
func NewSubagent(coordinator *agent.Coordinator, parentIDs ...string) tool.Handler {
	return subagentHandler{
		spawn:  NewDelegateTask(coordinator, parentIDs...),
		wait:   NewWaitAgent(coordinator),
		get:    NewGetAgent(coordinator),
		list:   NewListAgents(coordinator),
		cancel: NewCancelAgent(coordinator),
		resume: NewResumeAgent(coordinator),
	}
}

func (h subagentHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                   tool.NameSubagent,
		Description:            "Subagent capability for specialized concurrent work. Use action=spawn to delegate; completed results are delivered automatically to the owning turn. wait/get/list are diagnostic lifecycle inspection, while cancel/resume explicitly control existing work.",
		Kind:                   tool.KindAgent,
		Mutability:             tool.MutabilityMutating,
		Safety:                 tool.SafetyContract{MutationDomain: tool.MutationDomainAgentState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone},
		ExecutionTimeoutPolicy: tool.ExecutionTimeoutCallerBounded,
		Semantics:              h.callSemantics,
		InputSchema:            subagentInputSchema(),
		OutputSchema: map[string]any{
			"oneOf": []any{
				withActionSchema(subagentActionSpawn, delegateTaskOutputSchema()),
				withActionSchema(subagentActionWait, agentLifecycleOutputSchema(subagentActionWait)),
				withActionSchema(subagentActionGet, agentLifecycleOutputSchema(subagentActionGet)),
				withActionSchema(subagentActionList, agentLifecycleOutputSchema(subagentActionList)),
				withActionSchema(subagentActionCancel, agentLifecycleOutputSchema(subagentActionCancel)),
				withActionSchema(subagentActionResume, agentLifecycleOutputSchema(subagentActionResume)),
			},
		},
	}
}

func subagentInputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action":          map[string]any{"type": "string", "enum": []string{"spawn", "wait", "get", "list", "cancel", "resume"}, "description": "Operation: spawn delegates work with automatic result delivery; wait/get/list are diagnostic inspection; cancel/resume explicitly control existing work"},
			"task":            map[string]any{"type": "string", "description": "Task for action=spawn"},
			"profile":         map[string]any{"type": "string", "enum": agent.SubagentProfileNames(), "description": agent.SubagentProfileSchemaDescription()},
			"context":         map[string]any{"type": "string", "description": "Optional background context for action=spawn"},
			"timeout_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 86400, "description": "Optional child runtime timeout for spawn or bounded diagnostic timeout for wait"},
			"agent_id":        map[string]any{"type": "string", "description": "Target agent for get, cancel, or resume"},
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	}
}

func withActionSchema(action subagentAction, schema map[string]any) map[string]any {
	if schema == nil {
		return nil
	}
	clone := make(map[string]any, len(schema))
	for key, value := range schema {
		clone[key] = value
	}
	props, _ := clone["properties"].(map[string]any)
	nextProps := make(map[string]any, len(props)+1)
	for key, value := range props {
		nextProps[key] = value
	}
	nextProps["action"] = map[string]any{"type": "string", "enum": []string{string(action)}}
	clone["properties"] = nextProps
	required, _ := clone["required"].([]any)
	if required == nil {
		if raw, ok := clone["required"].([]string); ok {
			required = make([]any, 0, len(raw)+1)
			for _, item := range raw {
				required = append(required, item)
			}
		}
	}
	clone["required"] = append(required, "action")
	return clone
}

func (h subagentHandler) child(action string) (tool.Handler, bool) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "spawn":
		return h.spawn, true
	case "wait":
		return h.wait, true
	case "get":
		return h.get, true
	case "list":
		return h.list, true
	case "cancel":
		return h.cancel, true
	case "resume":
		return h.resume, true
	default:
		return nil, false
	}
}

func (h subagentHandler) callSemantics(arguments json.RawMessage) tool.CallSemantics {
	fallback := tool.CallSemantics{Mutability: tool.MutabilityMutating, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainAgentState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}}
	var input struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(arguments, &input) != nil {
		return fallback
	}
	child, ok := h.child(input.Action)
	if !ok {
		return fallback
	}
	return tool.StaticCallSemantics(child.Definition())
}

func (h subagentHandler) PermissionDetail(arguments json.RawMessage) string {
	action, childArgs, child, err := h.resolve(arguments)
	if err != nil {
		return ""
	}
	if provider, ok := child.(tool.DetailProvider); ok {
		if detail := strings.TrimSpace(provider.PermissionDetail(childArgs)); detail != "" {
			return detail
		}
	}
	return action
}

func (h subagentHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	action, childArgs, child, err := h.resolve(call.Arguments)
	if err != nil {
		return tool.Result{}, err
	}
	childCall := call
	childCall.Arguments = tool.NormalizeArguments(child.Definition(), childArgs)
	result, err := child.Execute(ctx, childCall)
	result.ToolName = call.Name
	if err == nil && len(result.StructuredOutput) > 0 {
		var payload map[string]any
		if json.Unmarshal(result.StructuredOutput, &payload) == nil {
			payload["action"] = action
			if encoded, encodeErr := json.Marshal(payload); encodeErr == nil {
				result.StructuredOutput = encoded
			}
		}
	}
	return result, err
}

func (h subagentHandler) resolve(arguments json.RawMessage) (string, json.RawMessage, tool.Handler, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(arguments, &object); err != nil {
		return "", nil, nil, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode subagent arguments", err)
	}
	var action string
	if raw, ok := object["action"]; ok {
		_ = json.Unmarshal(raw, &action)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	child, ok := h.child(action)
	if !ok {
		return "", nil, nil, tool.NewToolError(tool.ErrorCodeInvalidArguments, "subagent action must be spawn, wait, get, list, cancel, or resume")
	}
	delete(object, "action")
	childArgs, err := json.Marshal(object)
	if err != nil {
		return "", nil, nil, fmt.Errorf("encode subagent %s arguments: %w", action, err)
	}
	return action, childArgs, child, nil
}
