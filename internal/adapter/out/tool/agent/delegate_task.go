package agenttool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type delegateTaskHandler struct {
	coordinator *agent.Coordinator
	parentID    string
}

type delegateTaskInput struct {
	Task           string   `json:"task"`
	Profile        string   `json:"profile"`
	Context        string   `json:"context,omitempty"`
	DependsOn      []string `json:"depends_on,omitempty"`
	Optional       bool     `json:"optional,omitempty"`
	TimeoutSeconds int64    `json:"timeout_seconds,omitempty"`
}

// NewDelegateTask creates a tool.Handler that delegates a task to a specialized subagent.
func NewDelegateTask(coordinator *agent.Coordinator, parentIDs ...string) tool.Handler {
	var parentID string
	if len(parentIDs) > 0 {
		parentID = parentIDs[0]
	}
	return delegateTaskHandler{coordinator: coordinator, parentID: parentID}
}

func (delegateTaskHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                   tool.NameSubagent,
		Description:            "Spawn a specialized subagent asynchronously and return its agent_id immediately. Results required for the parent are delivered automatically. Use depends_on to gate a child on already-spawned children from the same parent turn. Set optional=true only for speculative work that must not block parent completion.",
		Kind:                   tool.KindAgent,
		Mutability:             tool.MutabilityMutating,
		Safety:                 tool.SafetyContract{MutationDomain: tool.MutationDomainAgentState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone},
		ExecutionTimeoutPolicy: tool.ExecutionTimeoutCallerBounded,
		PermissionDetailKey:    "task",
		OutputSchema:           delegateTaskOutputSchema(),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "Clear description of what the subagent should investigate or do.",
				},
				"profile": map[string]any{
					"type":        "string",
					"enum":        agent.SubagentProfileNames(),
					"description": agent.SubagentProfileSchemaDescription(),
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Optional background information, hints, or specific file paths to focus on.",
				},
				"depends_on": map[string]any{
					"type": "array", "maxItems": agent.MaxAgentDependencies, "items": map[string]any{"type": "string"},
					"description": "Agent IDs already spawned by this parent turn that must complete successfully before this child starts.",
				},
				"optional": map[string]any{
					"type":        "boolean",
					"description": "Speculative work that may be integrated if ready but does not block the parent final response and is canceled when the parent completes.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"maximum":     86400,
					"description": "Optional shorter execution timeout in seconds. Requests above the configured subagent maximum are clamped.",
				},
			},
			"required":             []string{"task", "profile"},
			"additionalProperties": false,
		},
	}
}

// PermissionDetail implements tool.DetailProvider so the permission modal displays
// a concise, formatted summary e.g. "[explorer] find authentication helpers".
func (h delegateTaskHandler) PermissionDetail(arguments json.RawMessage) string {
	var input delegateTaskInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return "subagent task"
	}
	profile := strings.TrimSpace(input.Profile)
	if profile == "" {
		profile = "subagent"
	}
	task := strings.TrimSpace(input.Task)
	if task == "" {
		task = "unspecified task"
	}
	if len(task) > 80 {
		task = task[:77] + "..."
	}
	return fmt.Sprintf("[%s] %s", profile, task)
}

func (h delegateTaskHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.coordinator == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "subagent coordinator is not configured")
	}

	var input delegateTaskInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode subagent spawn arguments", err)
	}

	task := strings.TrimSpace(input.Task)
	if task == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "task is required")
	}

	if input.TimeoutSeconds < 0 || input.TimeoutSeconds > 86400 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "timeout_seconds must be between 1 and 86400 when provided")
	}

	profile, err := agent.ParseSubagentProfile(input.Profile)
	if err != nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, err.Error())
	}

	turnRef := agent.TurnRefFromContext(ctx)
	if turnRef.TurnID == "" {
		turnRef.TurnID = h.parentID
	}
	req := agent.Request{
		SessionID: turnRef.SessionID,
		ParentID:  turnRef.TurnID,
		Profile:   profile,
		Task:      task,
		Context:   strings.TrimSpace(input.Context),
		DependsOn: append([]string(nil), input.DependsOn...),
		Optional:  input.Optional,
	}
	if input.TimeoutSeconds > 0 {
		req.Timeout = time.Duration(input.TimeoutSeconds) * time.Second
	}

	handle, err := h.coordinator.Spawn(ctx, req)
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "spawn subagent", err)
	}
	payload, err := json.Marshal(map[string]any{
		"agent_id": handle.ID,
		"profile":  handle.Profile,
		"status":   agent.StateQueued,
		"optional": input.Optional,
	})
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "encode subagent handle", err)
	}
	return tool.Result{
		CallID: call.ID, ToolName: call.Name,
		Output:           fmt.Sprintf("spawned %s · %s · %s", handle.ID, handle.Profile, agent.StateQueued),
		StructuredOutput: payload,
	}, nil
}
