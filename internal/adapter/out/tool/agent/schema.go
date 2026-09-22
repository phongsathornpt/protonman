package agenttool

import (
	"github.com/phongsathornpt/protonman/internal/core/agentprofile"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func agentStateSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []any{
		string(agent.StateQueued), string(agent.StateRunning), string(agent.StateCanceling),
		string(agent.StateCompleted), string(agent.StateFailed), string(agent.StateCanceled), string(agent.StateInterrupted),
		string(agent.StateResuming), string(agent.StateResumed),
	}}
}

func agentEvidenceSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"tool": map[string]any{"type": "string"}, "target": map[string]any{"type": "string"},
	}, "required": []any{"tool"}, "additionalProperties": false}
}

func agentFindingSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"claim":      map[string]any{"type": "string"},
		"confidence": map[string]any{"type": "string", "enum": []any{"high", "medium", "low"}},
		"evidence":   map[string]any{"type": "array", "items": agentEvidenceSchema()},
	}, "required": []any{"claim"}, "additionalProperties": false}
}

func agentVerificationSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"mutated": map[string]any{"type": "boolean"}, "verified": map[string]any{"type": "boolean"}, "verifier": map[string]any{"type": "string"},
	}, "required": []any{"mutated", "verified"}, "additionalProperties": false}
}

func agentResultSchema() map[string]any {
	result := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"conclusion":            map[string]any{"type": "string"},
			"findings":              map[string]any{"type": "array", "items": agentFindingSchema()},
			"blockers":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"summary":               map[string]any{"type": "string"},
			"provider":              map[string]any{"type": "string"},
			"model":                 map[string]any{"type": "string"},
			"rounds":                map[string]any{"type": "integer", "minimum": 0},
			"verification":          agentVerificationSchema(),
			"evidence":              map[string]any{"type": "array", "items": agentEvidenceSchema()},
			"changed_targets":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"queue_duration_ms":     map[string]any{"type": "integer", "minimum": 0},
			"execution_duration_ms": map[string]any{"type": "integer", "minimum": 0},
			"total_duration_ms":     map[string]any{"type": "integer", "minimum": 0},
			"error":                 map[string]any{"type": "string"},
		},
		"required":             []any{"conclusion", "findings", "blockers", "summary", "rounds", "verification", "evidence", "changed_targets", "queue_duration_ms", "execution_duration_ms", "total_duration_ms"},
		"additionalProperties": false,
	}
	return map[string]any{"oneOf": []any{result, map[string]any{"type": "null"}}}
}

func agentStatusSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"session_id":   map[string]any{"type": "string"},
			"id":           map[string]any{"type": "string"},
			"parent_id":    map[string]any{"type": "string"},
			"profile":      map[string]any{"type": "string", "enum": agentprofile.SubagentProfileNames()},
			"provider":     map[string]any{"type": "string"},
			"model":        map[string]any{"type": "string"},
			"task":         map[string]any{"type": "string"},
			"task_id":      map[string]any{"type": "string"},
			"depends_on":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"optional":     map[string]any{"type": "boolean"},
			"state":        agentStateSchema(),
			"version":      map[string]any{"type": "integer", "minimum": 1},
			"start_time":   map[string]any{"type": "string"},
			"started_at":   map[string]any{"type": "string"},
			"finished_at":  map[string]any{"type": "string"},
			"reason":       map[string]any{"type": "string"},
			"resumed_from": map[string]any{"type": "string"},
			"resumed_as":   map[string]any{"type": "string"},
		},
		"required":             []any{"version", "id", "profile", "task", "state", "start_time"},
		"additionalProperties": false,
	}
}

func delegateTaskOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agentId":  map[string]any{"type": "string"},
			"agent_id": map[string]any{"type": "string"},
			"profile":  map[string]any{"type": "string", "enum": agentprofile.SubagentProfileNames()},
			"status":   agentStateSchema(),
			"optional": map[string]any{"type": "boolean"},
			"taskId":   map[string]any{"type": "string"},
			"task_id":  map[string]any{"type": "string"},
		},
		"required":             []any{"profile", "status"},
		"additionalProperties": false,
	}
}

func agentLifecycleOutputSchema(action subagentAction) map[string]any {
	switch action {
	case tool.ActionWait:
		return map[string]any{"type": "object", "properties": map[string]any{
			"timed_out": map[string]any{"type": "boolean"},
			"event":     map[string]any{"type": []any{"object", "null"}},
			"events":    map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			"cursor":    map[string]any{"type": "integer", "minimum": 0},
			"truncated": map[string]any{"type": "boolean"},
		}, "required": []any{"timed_out", "event", "events", "cursor", "truncated"}, "additionalProperties": false}
	case tool.ActionResume:
		return resumeAgentOutputSchema()
	case tool.ActionGet, tool.ActionCancel:
		return map[string]any{"type": "object", "properties": map[string]any{
			"agent": agentStatusSchema(), "result": agentResultSchema(),
		}, "required": []any{"agent", "result"}, "additionalProperties": false}
	case tool.ActionList:
		return map[string]any{"type": "object", "properties": map[string]any{
			"agents": map[string]any{"type": "array", "items": agentStatusSchema()},
		}, "required": []any{"agents"}, "additionalProperties": false}
	default:
		return nil
	}
}

func resumeAgentOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"resumedFrom":  map[string]any{"type": "string"},
			"resumed_from": map[string]any{"type": "string"},
			"agentId":      map[string]any{"type": "string"},
			"agent_id":     map[string]any{"type": "string"},
			"profile":      map[string]any{"type": "string", "enum": agentprofile.SubagentProfileNames()},
			"status":       agentStateSchema(),
		},
		"required":             []any{"profile", "status"},
		"additionalProperties": false,
	}
}
