package agenttool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

type agentIDInput struct {
	AgentID string `json:"agent_id"`
}

type waitAgentInput struct {
	TimeoutSeconds int64 `json:"timeout_seconds,omitempty"`
}

type agentLifecycleHandler struct {
	action      subagentAction
	coordinator *agent.Coordinator
}

func NewWaitAgent(c *agent.Coordinator) tool.Handler {
	return agentLifecycleHandler{action: subagentActionWait, coordinator: c}
}
func NewGetAgent(c *agent.Coordinator) tool.Handler {
	return agentLifecycleHandler{action: subagentActionGet, coordinator: c}
}
func NewListAgents(c *agent.Coordinator) tool.Handler {
	return agentLifecycleHandler{action: subagentActionList, coordinator: c}
}
func NewCancelAgent(c *agent.Coordinator) tool.Handler {
	return agentLifecycleHandler{action: subagentActionCancel, coordinator: c}
}
func NewResumeAgent(c *agent.Coordinator) tool.Handler {
	return agentLifecycleHandler{action: subagentActionResume, coordinator: c}
}

func (h agentLifecycleHandler) Definition() tool.Definition {
	def := tool.Definition{Name: tool.NameSubagent, Kind: tool.KindAgent, ExecutionTimeoutPolicy: tool.ExecutionTimeoutCallerBounded}
	switch h.action {
	case subagentActionWait:
		def.Description = "Wait for the next subagent completion/failure activity. A wait timeout is non-fatal and never cancels children."
		def.Mutability = tool.MutabilityReadOnly
		def.Safety = tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}
		def.InputSchema = map[string]any{"type": "object", "properties": map[string]any{
			"timeout_seconds": map[string]any{"type": "integer", "minimum": 0, "maximum": 3600},
		}, "additionalProperties": false}
	case subagentActionGet:
		def.Description = "Inspect one retained subagent and its terminal result when available."
		def.Mutability = tool.MutabilityReadOnly
		def.Safety = tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}
		def.PermissionDetailKey = "agent_id"
		def.InputSchema = agentIDSchema()
	case subagentActionList:
		def.Description = "List retained subagents and their lifecycle states."
		def.Mutability = tool.MutabilityReadOnly
		def.Safety = tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}
		def.InputSchema = tool.NoArgumentsSchema()
	case subagentActionResume:
		def.Description = "Explicitly restart an interrupted retained subagent as a fresh child after re-checking current workspace state."
		def.Mutability = tool.MutabilityMutating
		def.Safety = tool.SafetyContract{MutationDomain: tool.MutationDomainAgentState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}
		def.PermissionDetailKey = "agent_id"
		def.InputSchema = agentIDSchema()
	case subagentActionCancel:
		def.Description = "Explicitly cancel a queued or running subagent."
		def.Mutability = tool.MutabilityMutating
		def.Safety = tool.SafetyContract{MutationDomain: tool.MutationDomainAgentState, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyNone}
		def.PermissionDetailKey = "agent_id"
		def.InputSchema = agentIDSchema()
	}
	def.OutputSchema = agentLifecycleOutputSchema(h.action)
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
	switch h.action {
	case subagentActionWait:
		return h.wait(ctx, call)
	case subagentActionGet:
		return h.get(ctx, call)
	case subagentActionList:
		return h.list(ctx, call)
	case subagentActionResume:
		return h.resume(ctx, call)
	case subagentActionCancel:
		return h.cancel(ctx, call)
	default:
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "unknown agent lifecycle handler")
	}
}

func (h agentLifecycleHandler) wait(ctx context.Context, call tool.Call) (tool.Result, error) {
	var in waitAgentInput
	if err := json.Unmarshal(call.Arguments, &in); err != nil {
		return tool.Result{}, invalidArgs("decode subagent wait arguments", err)
	}
	if in.TimeoutSeconds < 0 || in.TimeoutSeconds > 3600 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "timeout_seconds must be between 0 and 3600 when provided")
	}
	var timeout time.Duration
	if in.TimeoutSeconds > 0 {
		timeout = time.Duration(in.TimeoutSeconds) * time.Second
		if timeout < 10*time.Second {
			timeout = 10 * time.Second
		}
	}
	turnRef := agent.TurnRefFromContext(ctx)
	wr, err := h.coordinator.WaitActivityForTurn(ctx, turnRef, timeout)
	if err != nil {
		return tool.Result{}, classifyAgentError("wait for subagent activity", err)
	}
	summary := "wait timed out"
	if wr.Event != nil {
		summary = fmt.Sprintf("%s · %s", wr.Event.AgentID, wr.Event.Kind)
	}
	return agentJSONResult(call, summary, map[string]any{
		"timed_out": wr.TimedOut, "event": wr.Event, "events": wr.Events,
		"cursor": wr.Cursor, "truncated": wr.Truncated, "agents": wr.Agents,
	})
}

func (h agentLifecycleHandler) get(ctx context.Context, call tool.Call) (tool.Result, error) {
	id, err := decodeAgentID(call)
	if err != nil {
		return tool.Result{}, err
	}
	status, result, ok := h.coordinator.LookupRef(agent.AgentRef{SessionID: agent.SessionIDFromContext(ctx), AgentID: id})
	if !ok {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeNotFound, fmt.Sprintf("subagent %q not found", id))
	}
	return agentJSONResult(call, fmt.Sprintf("%s · %s", status.ID, status.State), map[string]any{"agent": status, "result": resultPayload(result)})
}

func (h agentLifecycleHandler) list(ctx context.Context, call tool.Call) (tool.Result, error) {
	agents := h.coordinator.ListSession(agent.SessionIDFromContext(ctx))
	return agentJSONResult(call, fmt.Sprintf("%d retained agents", len(agents)), map[string]any{"agents": agents})
}

func (h agentLifecycleHandler) resume(ctx context.Context, call tool.Call) (tool.Result, error) {
	id, err := decodeAgentID(call)
	if err != nil {
		return tool.Result{}, err
	}
	turnRef := agent.TurnRefFromContext(ctx)
	handle, err := h.coordinator.ResumeRef(ctx, agent.AgentRef{SessionID: turnRef.SessionID, AgentID: id}, turnRef)
	if err != nil {
		return tool.Result{}, classifyAgentError("resume subagent", err)
	}
	status, _ := h.coordinator.GetRef(agent.AgentRef{SessionID: turnRef.SessionID, AgentID: handle.ID})
	return agentJSONResult(call, fmt.Sprintf("resumed %s as %s", id, handle.ID), map[string]any{
		"resumed_from": id, "agent_id": handle.ID, "profile": handle.Profile, "status": status.State,
	})
}

func (h agentLifecycleHandler) cancel(ctx context.Context, call tool.Call) (tool.Result, error) {
	id, err := decodeAgentID(call)
	if err != nil {
		return tool.Result{}, err
	}
	if err := h.coordinator.CancelRef(agent.AgentRef{SessionID: agent.SessionIDFromContext(ctx), AgentID: id}); err != nil {
		return tool.Result{}, classifyAgentError("cancel subagent", err)
	}
	status, result, _ := h.coordinator.LookupRef(agent.AgentRef{SessionID: agent.SessionIDFromContext(ctx), AgentID: id})
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
	if errors.Is(err, agent.ErrNotResumable) {
		return tool.WrapToolError(tool.ErrorCodeConflict, message, err)
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
	evidence := result.Evidence
	if evidence == nil {
		evidence = []agent.EvidenceRef{}
	}
	changedTargets := result.ChangedTargets
	if changedTargets == nil {
		changedTargets = []string{}
	}
	payload := map[string]any{
		"summary": result.Summary, "rounds": result.Rounds,
		"verification": result.Verification, "evidence": evidence, "changed_targets": changedTargets,
		"queue_duration_ms": result.QueueDuration.Milliseconds(), "execution_duration_ms": result.Duration.Milliseconds(), "total_duration_ms": result.TotalDuration.Milliseconds(),
	}
	if result.Provider != "" {
		payload["provider"] = result.Provider
	}
	if result.Model != "" {
		payload["model"] = result.Model
	}
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
