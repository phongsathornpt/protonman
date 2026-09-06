package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/tool"
)

type delegateTaskHandler struct {
	coordinator *agent.Coordinator
	parentID    string
}

type delegateTaskInput struct {
	Task    string `json:"task"`
	Profile string `json:"profile"`
	Context string `json:"context,omitempty"`
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
		Name:                   "delegate_task",
		Description:            "Delegate an investigation, code review, or targeted task to a specialized subagent running in the background.",
		Kind:                   tool.KindRead,
		Mutability:             tool.MutabilityMutating,
		ExecutionTimeoutPolicy: tool.ExecutionTimeoutCallerBounded,
		PermissionDetailKey:    "task",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]any{
					"type":        "string",
					"description": "Clear description of what the subagent should investigate or do.",
				},
				"profile": map[string]any{
					"type":        "string",
					"enum":        []string{"explorer", "reviewer", "worker", "pow", "dex", "int"},
					"description": "The subagent profile: 'explorer' (read-only search & inspection), 'reviewer' (read-only code & security review), 'worker' (code modifications and commands), 'pow' (high-velocity pragmatic execution with capacity-limited 1-line re-encoding), 'dex' (defensive zero-regression engineering with empirical escape & named verifiers), or 'int' (deep architectural reasoning with broadcast hub & bridge-before-conclusion).",
				},
				"context": map[string]any{
					"type":        "string",
					"description": "Optional background information, hints, or specific file paths to focus on.",
				},
			},
			"required": []string{"task", "profile"},
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
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode delegate_task arguments", err)
	}

	task := strings.TrimSpace(input.Task)
	if task == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "task is required")
	}

	profile, err := agent.ParseProfile(input.Profile)
	if err != nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, err.Error())
	}

	req := agent.Request{
		ParentID: h.parentID,
		Profile:  profile,
		Task:     task,
		Context:  strings.TrimSpace(input.Context),
		Depth:    0,
	}

	res, err := h.coordinator.Run(ctx, req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeCanceled, "subagent execution canceled")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return tool.Result{}, tool.NewToolError(tool.ErrorCodeDeadlineExceeded, "subagent execution timed out")
		}
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "subagent execution failed", err)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<subagent_result id=%q profile=%q rounds=%d>\n", res.AgentID, res.Profile, res.Rounds))
	b.WriteString(res.Summary)
	b.WriteString("\n</subagent_result>")

	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   b.String(),
	}, nil
}
