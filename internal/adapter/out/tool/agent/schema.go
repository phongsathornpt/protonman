package agenttool

import "github.com/projectTHORN/proton/internal/feature/agent"

func agentStateSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []any{
		string(agent.StateQueued), string(agent.StateRunning), string(agent.StateCanceling),
		string(agent.StateCompleted), string(agent.StateFailed), string(agent.StateCanceled),
	}}
}

func agentResultSchema() map[string]any {
	result := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary":               map[string]any{"type": "string"},
			"rounds":                map[string]any{"type": "integer", "minimum": 0},
			"queue_duration_ms":     map[string]any{"type": "integer", "minimum": 0},
			"execution_duration_ms": map[string]any{"type": "integer", "minimum": 0},
			"total_duration_ms":     map[string]any{"type": "integer", "minimum": 0},
			"error":                 map[string]any{"type": "string"},
		},
		"required":             []any{"summary", "rounds", "queue_duration_ms", "execution_duration_ms", "total_duration_ms"},
		"additionalProperties": false,
	}
	return map[string]any{"oneOf": []any{result, map[string]any{"type": "null"}}}
}

func agentStatusSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id":          map[string]any{"type": "string"},
			"parent_id":   map[string]any{"type": "string"},
			"profile":     map[string]any{"type": "string", "enum": agent.ProfileNames()},
			"task":        map[string]any{"type": "string"},
			"state":       agentStateSchema(),
			"start_time":  map[string]any{"type": "string"},
			"started_at":  map[string]any{"type": "string"},
			"finished_at": map[string]any{"type": "string"},
			"reason":      map[string]any{"type": "string"},
		},
		"required":             []any{"id", "profile", "task", "state", "start_time"},
		"additionalProperties": false,
	}
}

func delegateTaskOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{"type": "string"},
			"profile":  map[string]any{"type": "string", "enum": agent.ProfileNames()},
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
			"agent_id": map[string]any{"type": "string"}, "status": agentStateSchema(), "result": agentResultSchema(),
		}, "required": []any{"agent_id", "status", "result"}, "additionalProperties": false}
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
