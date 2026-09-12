package turn

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

// progressSafetyBudget bounds tool churn without treating productive work as
// a consumable quota. Meaningful successful work resets the stagnant window;
// an independent emergency ceiling still bounds pathological unique-call loops.
type progressSafetyBudget struct {
	totalCalls        int
	stagnantCalls     int
	maxStagnantCalls  int
	emergencyMaxCalls int
}

func newProgressSafetyBudget(maxStagnantCalls, emergencyMaxCalls int) *progressSafetyBudget {
	return &progressSafetyBudget{
		maxStagnantCalls:  maxStagnantCalls,
		emergencyMaxCalls: emergencyMaxCalls,
	}
}

func (b *progressSafetyBudget) observeRound(executions []executedCall, definitions map[string]tool.Definition) bool {
	if b == nil || len(executions) == 0 {
		return false
	}
	b.totalCalls += len(executions)
	progressed := false
	for _, execution := range executions {
		definition, ok := definitions[execution.call.Name]
		if ok && executionAdvancesSafetyBudget(definition, execution) {
			progressed = true
		}
	}
	if progressed {
		b.stagnantCalls = 0
	} else {
		b.stagnantCalls += len(executions)
	}
	return progressed
}

func (b *progressSafetyBudget) exhausted() bool {
	if b == nil {
		return false
	}
	return (b.maxStagnantCalls > 0 && b.stagnantCalls >= b.maxStagnantCalls) ||
		(b.emergencyMaxCalls > 0 && b.totalCalls >= b.emergencyMaxCalls)
}

func (b *progressSafetyBudget) remainingHardLimit() int {
	if b == nil || b.emergencyMaxCalls <= 0 {
		return 0
	}
	remaining := b.emergencyMaxCalls - b.totalCalls
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (b *progressSafetyBudget) shouldWarn(warned bool) bool {
	if b == nil || warned || b.maxStagnantCalls <= 0 || b.stagnantCalls <= 0 {
		return false
	}
	return b.stagnantCalls*5 >= b.maxStagnantCalls*3
}

func (b *progressSafetyBudget) exhaustedReason() string {
	if b == nil {
		return ""
	}
	if b.emergencyMaxCalls > 0 && b.totalCalls >= b.emergencyMaxCalls {
		return "emergency_tool_ceiling"
	}
	if b.maxStagnantCalls > 0 && b.stagnantCalls >= b.maxStagnantCalls {
		return "stagnant_tool_budget"
	}
	return ""
}

func executionAdvancesSafetyBudget(definition tool.Definition, execution executedCall) bool {
	if execution.suppressed || execution.err != nil || execution.result.Failure != nil || execution.result.Denied {
		return false
	}
	if definition.Kind == tool.KindTask {
		return false
	}
	if definition.Kind == tool.KindAgent {
		action := strings.ToLower(tool.ExtractString(execution.call.ArgumentsMap(), "action"))
		switch action {
		case tool.ActionSpawn, tool.ActionResume, tool.ActionCancel:
			return true
		default:
			return false
		}
	}
	return true
}
