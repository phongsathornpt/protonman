package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/agent"
	"github.com/projectTHORN/proton/internal/tool"
)

type agentIDInput struct {
	AgentID string `json:"agent_id"`
}

type waitAgentInput struct {
	AgentID        string `json:"agent_id"`
	TimeoutSeconds int64  `json:"timeout_seconds,omitempty"`
}

type agentLifecycleHandler struct {
	name        string
	coordinator *agent.Coordinator
}

func NewWaitAgent(c *agent.Coordinator) tool.Handler {
	return agentLifecycleHandler{name: "wait_agent", coordinator: c}
}
func NewGetAgent(c *agent.Coordinator) tool.Handler {
	return agentLifecycleHandler{name: "get_agent", coordinator: c}
}
func NewListAgents(c *agent.Coordinator) tool.Handler {
	return agentLifecycleHandler{name: "list_agents", coordinator: c}
}
func NewCancelAgent(c *agent.Coordinator) tool.Handler {
	return agentLifecycleHandler{name: "cancel_agent", coordinator: c}
}

func (h agentLifecycleHandler) Definition() tool.Definition {
	def := tool.Definition{Name: h.name, Kind: tool.KindForName(h.name), ExecutionTimeoutPolicy: tool.ExecutionTimeoutCallerBounded}
	switch h.name {
	case "wait_agent":
		def.Description = "Wait briefly for a subagent without canceling it when the wait expires."
		def.Mutability = tool.MutabilityReadOnly
		def.Safety = tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}
		def.PermissionDetailKey = "agent_id"
		def.InputSchema = map[string]any{"type": "object", "properties": map[string]any{
			"agent_id":        map[string]any{"type": "string"},
			"timeout_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 300},
		}, "required": []string{"agent_id"}, "additionalProperties": false}
	case "get_agent":
		def.Description = "Inspect one retained subagent and its terminal result when available."
		def.Mutability = tool.MutabilityReadOnly
		def.Safety = tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}
		def.PermissionDetailKey = "agent_id"
		def.InputSchema = agentIDSchema()
	case "list_agents":
		def.Description = "List retained subagents and their lifecycle states."
		def.Mutability = tool.MutabilityReadOnly
		def.Safety = tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}
		def.InputSchema = tool.NoArgumentsSchema()
	case "cancel_agent":
		def.Description = "Explicitly cancel a queued or running subagent."
		def.Mutability = tool.MutabilityMutating
		def.Safety = tool.SafetyContract{MutationDomain: tool.MutationDomainAgentState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}
		def.PermissionDetailKey = "agent_id"
		def.InputSchema = agentIDSchema()
	}
	def.OutputSchema = agentLifecycleOutputSchema(h.name)
	return def
}

func agentIDSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"agent_id": map[string]any{"type": "string"},
	}, "required": []string{"agent_id"}, "additionalProperties": false}
}

func (h agentLifecycleHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	if h.coordinator == nil {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "subagent coordinator is not configured")
	}
	switch h.name {
	case "wait_agent":
		return h.wait(ctx, call)
	case "get_agent":
		return h.get(call)
	case "list_agents":
		return h.list(call)
	case "cancel_agent":
		return h.cancel(call)
	default:
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "unknown agent lifecycle handler")
	}
}

func (h agentLifecycleHandler) wait(ctx context.Context, call tool.Call) (tool.Result, error) {
	var in waitAgentInput
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return tool.Result{}, invalidArgs("decode wait_agent arguments", err)
	}
	id := strings.TrimSpace(in.AgentID)
	if id == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "agent_id is required")
	}
	if in.TimeoutSeconds < 0 || in.TimeoutSeconds > 300 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "timeout_seconds must be between 1 and 300 when provided")
	}
	var timeout time.Duration
	if in.TimeoutSeconds > 0 {
		timeout = time.Duration(in.TimeoutSeconds) * time.Second
	}
	wr, err := h.coordinator.Wait(ctx, id, timeout)
	if err != nil {
		return tool.Result{}, classifyAgentError("wait for subagent", err)
	}
	return agentJSONResult(call, fmt.Sprintf("%s · %s", id, wr.State), map[string]any{"agent_id": id, "status": wr.State, "result": resultPayload(wr.Result)})
}

func (h agentLifecycleHandler) get(call tool.Call) (tool.Result, error) {
	id, err := decodeAgentID(call)
	if err != nil {
		return tool.Result{}, err
	}
	status, result, ok := h.coordinator.Lookup(id)
	if !ok {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeNotFound, fmt.Sprintf("subagent %q not found", id))
	}
	return agentJSONResult(call, fmt.Sprintf("%s · %s", status.ID, status.State), map[string]any{"agent": status, "result": resultPayload(result)})
}

func (h agentLifecycleHandler) list(call tool.Call) (tool.Result, error) {
	agents := h.coordinator.List()
	return agentJSONResult(call, fmt.Sprintf("%d retained agents", len(agents)), map[string]any{"agents": agents})
}

func (h agentLifecycleHandler) cancel(call tool.Call) (tool.Result, error) {
	id, err := decodeAgentID(call)
	if err != nil {
		return tool.Result{}, err
	}
	if err := h.coordinator.Cancel(id); err != nil {
		return tool.Result{}, classifyAgentError("cancel subagent", err)
	}
	status, result, _ := h.coordinator.Lookup(id)
	return agentJSONResult(call, fmt.Sprintf("cancel requested · %s · %s", status.ID, status.State), map[string]any{"agent": status, "result": resultPayload(result)})
}

func decodeAgentID(call tool.Call) (string, error) {
	var in agentIDInput
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return "", invalidArgs("decode agent arguments", err)
	}
	id := strings.TrimSpace(in.AgentID)
	if id == "" {
		return "", tool.NewToolError(tool.ErrorCodeInvalidArguments, "agent_id is required")
	}
	return id, nil
}

func invalidArgs(message string, err error) error {
	return tool.WrapToolError(tool.ErrorCodeInvalidArguments, message, err)
}

func classifyAgentError(message string, err error) error {
	if errors.Is(err, agent.ErrNotFound) {
		return tool.WrapToolError(tool.ErrorCodeNotFound, message, err)
	}
	if errors.Is(err, context.Canceled) {
		return tool.WrapToolError(tool.ErrorCodeCanceled, message, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return tool.WrapToolError(tool.ErrorCodeDeadlineExceeded, message, err)
	}
	return tool.WrapToolError(tool.ErrorCodeExecution, message, err)
}

func resultPayload(result *agent.Result) any {
	if result == nil {
		return nil
	}
	payload := map[string]any{"summary": result.Summary, "rounds": result.Rounds, "queue_duration_ms": result.QueueDuration.Milliseconds(), "execution_duration_ms": result.Duration.Milliseconds(), "total_duration_ms": result.TotalDuration.Milliseconds()}
	if result.Err != nil {
		payload["error"] = result.Err.Error()
	}
	return payload
}

func agentJSONResult(call tool.Call, summary string, payload any) (tool.Result, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeExecution, "encode agent result", err)
	}
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: summary, StructuredOutput: encoded}, nil
}
