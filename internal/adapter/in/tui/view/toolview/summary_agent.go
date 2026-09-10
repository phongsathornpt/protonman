package toolview

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func summarizeAgentTool(name, body string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(body), &payload) != nil {
		return "agent updated"
	}
	action, _ := payload["action"].(string)
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		call := tool.Call{Name: strings.TrimSpace(name), Arguments: json.RawMessage(`{}`)}
		if call.Name == "subagent" {
			action = strings.ToLower(strings.TrimSpace(tool.ExtractString(call.ArgumentsMap(), "action")))
		}
	}
	if action == "list" {
		agents, _ := payload["agents"].([]any)
		active := 0
		for _, raw := range agents {
			if m, ok := raw.(map[string]any); ok {
				if st, _ := m["state"].(string); st == "queued" || st == "running" || st == "canceling" {
					active++
				}
			}
		}
		return fmt.Sprintf("%d agents · %d active", len(agents), active)
	}
	id, _ := payload["agent_id"].(string)
	status, _ := payload["status"].(string)
	if agentObj, ok := payload["agent"].(map[string]any); ok {
		if id == "" {
			id, _ = agentObj["id"].(string)
		}
		if status == "" {
			status, _ = agentObj["state"].(string)
		}
	}
	resultSummary := agentResultSummary(payload)
	switch action {
	case "spawn":
		if id != "" {
			return fmt.Sprintf("spawned %s · %s", id, status)
		}
		return "subagent spawned"
	case "wait":
		if timedOut, _ := payload["timed_out"].(bool); timedOut {
			return "no new agent activity"
		}
		events, _ := payload["events"].([]any)
		if len(events) > 1 {
			return fmt.Sprintf("%d agent lifecycle events", len(events))
		}
		if event, ok := payload["event"].(map[string]any); ok {
			eventID, _ := event["agent_id"].(string)
			eventKind, _ := event["kind"].(string)
			if eventID != "" || eventKind != "" {
				return strings.Trim(strings.Join([]string{eventID, eventKind}, " · "), " ·")
			}
		}
		return "agent activity received"
	case "get":
		return joinAgentCompletionSummary(id, status, resultSummary)
	case "cancel":
		return fmt.Sprintf("cancel requested · %s", id)
	case "resume":
		from, _ := payload["resumed_from"].(string)
		if from != "" && id != "" {
			return fmt.Sprintf("resumed %s as %s · %s", from, id, status)
		}
		return joinAgentCompletionSummary(id, status, resultSummary)
	default:
		if id != "" {
			return fmt.Sprintf("%s · %s", id, status)
		}
	}
	return "agent updated"
}

func agentResultSummary(payload map[string]any) string {
	result, _ := payload["result"].(map[string]any)
	if result == nil {
		return ""
	}
	summary, _ := result["summary"].(string)
	return textview.TruncateEllipsis(strings.TrimSpace(summary), 96)
}

func joinAgentCompletionSummary(id, status, summary string) string {
	base := strings.TrimSpace(strings.Join([]string{id, status}, " · "))
	if summary == "" {
		return base
	}
	if base == "" {
		return summary
	}
	return base + " · " + summary
}
