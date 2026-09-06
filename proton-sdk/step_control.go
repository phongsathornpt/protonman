package protonsdk

// StepContext is the provider-neutral state exposed to agent stop policies.
type StepContext struct {
	StepNumber int
	Messages   []Message
	LastResult StepResult
}

// StopCondition decides whether an agent loop should stop before another model step.
type StopCondition func(StepContext) bool

// StopAfterSteps returns a stop condition that fires once the step count reaches max.
func StopAfterSteps(max int) StopCondition {
	return func(ctx StepContext) bool {
		return max > 0 && ctx.StepNumber >= max
	}
}

// StopWhenNoToolCalls stops when the last model step did not request tools.
func StopWhenNoToolCalls() StopCondition {
	return func(ctx StepContext) bool {
		return len(ctx.LastResult.ToolCalls) == 0
	}
}

// ShouldStop returns true when any supplied condition requests termination.
func ShouldStop(ctx StepContext, conditions ...StopCondition) bool {
	for _, condition := range conditions {
		if condition != nil && condition(ctx) {
			return true
		}
	}
	return false
}
