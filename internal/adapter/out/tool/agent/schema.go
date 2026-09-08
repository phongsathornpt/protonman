package agenttool

import "github.com/phongsathornpt/protonman/internal/feature/agent"

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

func agentVerificationSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"mutated": map[string]any{"type": "boolean"}, "verified": map[string]any{"type": "boolean"}, "verifier": map[string]any{"type": "string"},
	}, "required": []any{"mutated", "verified"}, "additionalProperties": false}
}

func agentResultSchema() map[string]any {
	result := map[string]any{
		"type": "object",
		"properties": map[string]any{
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
		"required":             []any{"summary", "rounds", "verification", "evidence", "changed_targets", "queue_duration_ms", "execution_duration_ms", "total_duration_ms"},
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
			"profile":      map[string]any{"type": "string", "enum": agent.SubagentProfileNames()},
			"provider":     map[string]any{"type": "string"},
			"model":        map[string]any{"type": "string"},
			"task":         map[string]any{"type": "string"},
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
			"agent_id": map[string]any{"type": "string"},
			"profile":  map[string]any{"type": "string", "enum": agent.SubagentProfileNames()},
			"status":   agentStateSchema(),
		},
		"required":             []any{"agent_id", "profile", "status"},
		"additionalProperties": false,
	}
}

func agentLifecycleOutputSchema(name string) map[string]any {
	switch name {
	case "wait_agent":
		return map[string]any{"type": "object", "properties": map[string]any{
			"timed_out": map[string]any{"type": "boolean"},
			"event":     map[string]any{"type": []any{"object", "null"}},
			"agents":    map[string]any{"type": "array", "items": agentStatusSchema()},
		}, "required": []any{"timed_out", "event", "agents"}, "additionalProperties": false}
	case "resume_agent":
		return resumeAgentOutputSchema()
	case "get_agent", "cancel_agent":
		return map[string]any{"type": "object", "properties": map[string]any{
			"agent": agentStatusSchema(), "result": agentResultSchema(),
		}, "required": []any{"agent", "result"}, "additionalProperties": false}
	case "list_agents":
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
			"resumed_from": map[string]any{"type": "string"},
			"agent_id":     map[string]any{"type": "string"},
			"profile":      map[string]any{"type": "string", "enum": agent.SubagentProfileNames()},
			"status":       agentStateSchema(),
		},
		"required":             []any{"resumed_from", "agent_id", "profile", "status"},
		"additionalProperties": false,
	}
}
