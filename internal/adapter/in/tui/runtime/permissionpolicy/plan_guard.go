package permissionpolicy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// EvaluatePlanModeCall validates whether a tool call request is permitted under read-only plan mode.
func EvaluatePlanModeCall(request permission.Request) error {
	switch request.ToolKind {
	case permission.ToolRead, permission.ToolGrep, permission.ToolWeb, permission.ToolTask:
		return nil
	case permission.ToolBash:
		var input struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(request.Arguments, &input) == nil && tool.AnalyzeCommand(input.Command).Effect == tool.CommandEffectReadOnly {
			return nil
		}
	case permission.ToolAgent:
		if request.ToolName == "subagent" {
			var input struct {
				Action string `json:"action"`
			}
			if json.Unmarshal(request.Arguments, &input) == nil && (input.Action == "wait" || input.Action == "get" || input.Action == "list") {
				return nil
			}
		}
	}
	return fmt.Errorf("plan mode is read-only; %s tool %q is blocked", request.ToolKind, request.ToolName)
}

// NewPlanModeGuard returns a CallGuard function that rejects mutating actions while active.
func NewPlanModeGuard(isActive func() bool) func(context.Context, permission.Request) error {
	return func(_ context.Context, request permission.Request) error {
		if isActive != nil && !isActive() {
			return nil
		}
		return EvaluatePlanModeCall(request)
	}
}
