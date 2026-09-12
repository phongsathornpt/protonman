package toolview

import (
	"encoding/json"
	"fmt"
	"strings"
)

func summarizeTodoSnapshot(body string) string {
	var payload struct {
		Revision uint64            `json:"revision"`
		Items    []json.RawMessage `json:"items"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return "task snapshot"
	}
	return fmt.Sprintf("Tasks %d · revision %d", len(payload.Items), payload.Revision)
}

func summarizeTodoUpdate(body string) string {
	var payload struct {
		Total      int `json:"total"`
		Completed  int `json:"completed"`
		InProgress int `json:"in_progress"`
		Changes    struct {
			Added     int `json:"added"`
			Removed   int `json:"removed"`
			Updated   int `json:"updated"`
			Started   int `json:"started"`
			Completed int `json:"completed"`
			Reopened  int `json:"reopened"`
		} `json:"changes"`
	}
	if json.Unmarshal([]byte(body), &payload) != nil {
		return "tasks updated"
	}
	parts := []string{"Tasks updated"}
	if payload.Changes.Completed > 0 {
		parts = append(parts, fmt.Sprintf("%d completed", payload.Changes.Completed))
	}
	if payload.Changes.Started > 0 {
		parts = append(parts, fmt.Sprintf("%d started", payload.Changes.Started))
	}
	if payload.Changes.Updated > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", payload.Changes.Updated))
	}
	if payload.Changes.Added > 0 {
		parts = append(parts, fmt.Sprintf("%d added", payload.Changes.Added))
	}
	if payload.Changes.Removed > 0 {
		parts = append(parts, fmt.Sprintf("%d removed", payload.Changes.Removed))
	}
	if payload.Changes.Reopened > 0 {
		parts = append(parts, fmt.Sprintf("%d reopened", payload.Changes.Reopened))
	}
	if len(parts) > 1 {
		return strings.Join(parts, " · ")
	}
	summary := fmt.Sprintf("Tasks updated · %d/%d complete", payload.Completed, payload.Total)
	if payload.InProgress > 0 {
		summary += fmt.Sprintf(" · %d active", payload.InProgress)
	}
	return summary
}
