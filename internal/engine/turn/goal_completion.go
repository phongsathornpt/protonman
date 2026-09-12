package turn

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type taskPlanProgress struct {
	observed   bool
	total      int
	pending    int
	inProgress int
	completed  int
}

func (p *taskPlanProgress) observe(executions []executedCall) {
	for _, execution := range executions {
		if execution.call.Name != tool.NameTodo || execution.err != nil || execution.result.Denied || execution.result.Failure != nil || len(execution.result.StructuredOutput) == 0 {
			continue
		}
		var envelope struct {
			Items      json.RawMessage `json:"items"`
			Total      *int            `json:"total"`
			Pending    *int            `json:"pending"`
			InProgress *int            `json:"in_progress"`
			Completed  *int            `json:"completed"`
		}
		if err := json.Unmarshal(execution.result.StructuredOutput, &envelope); err != nil {
			continue
		}
		if envelope.Total != nil && envelope.Pending != nil && envelope.InProgress != nil && envelope.Completed != nil {
			p.observed = true
			p.total, p.pending, p.inProgress, p.completed = *envelope.Total, *envelope.Pending, *envelope.InProgress, *envelope.Completed
			continue
		}
		if len(envelope.Items) == 0 || string(envelope.Items) == "null" {
			continue
		}
		var items []struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(envelope.Items, &items); err != nil {
			continue
		}
		p.observed = true
		p.total, p.pending, p.inProgress, p.completed = len(items), 0, 0, 0
		for _, item := range items {
			switch strings.TrimSpace(item.Status) {
			case "pending":
				p.pending++
			case "in_progress":
				p.inProgress++
			case "completed":
				p.completed++
			}
		}
	}
}

func (p taskPlanProgress) complete() bool {
	return p.observed && p.total > 0 && p.pending == 0 && p.inProgress == 0 && p.completed == p.total
}

func (p taskPlanProgress) incomplete() bool {
	return p.observed && p.total > 0 && !p.complete()
}

func (l *Loop) hasActiveGoal() bool {
	return l != nil && l.promptSpec != nil && strings.TrimSpace(l.promptSpec.ActiveGoal) != ""
}

func (l *Loop) goalCompleted(plan taskPlanProgress, verification VerificationState) bool {
	if !l.hasActiveGoal() || !plan.complete() {
		return false
	}
	return !verification.Mutated || verification.Verified
}

func goalProgressPrompt(plan taskPlanProgress, verification VerificationState) string {
	if plan.incomplete() {
		return fmt.Sprintf("ACTIVE GOAL PROGRESS\nThe tracked task plan is not complete: %d pending, %d in progress, %d completed of %d. Continue concrete work instead of finalizing the active goal.", plan.pending, plan.inProgress, plan.completed, plan.total)
	}
	if plan.complete() && verification.Mutated && !verification.Verified {
		return "ACTIVE GOAL VERIFICATION REQUIRED\nThe tracked task plan is complete, but successful workspace mutations have not been empirically verified after the latest mutation. Run an appropriate verifier before finalizing the active goal."
	}
	return ""
}
