package protonsdk

import "testing"

func TestStopAfterSteps(t *testing.T) {
	stop := StopAfterSteps(3)
	if stop(StepContext{StepNumber: 2}) {
		t.Fatal("stopped too early")
	}
	if !stop(StepContext{StepNumber: 3}) {
		t.Fatal("expected stop at limit")
	}
}

func TestStopWhenNoToolCalls(t *testing.T) {
	stop := StopWhenNoToolCalls()
	if !stop(StepContext{}) {
		t.Fatal("expected stop without tool calls")
	}
	if stop(StepContext{LastResult: StepResult{ToolCalls: []ToolCall{{ID: "1", Name: "tool"}}}}) {
		t.Fatal("unexpected stop with tool call")
	}
}

func TestShouldStopUsesAnyCondition(t *testing.T) {
	ctx := StepContext{StepNumber: 2, LastResult: StepResult{ToolCalls: []ToolCall{{ID: "1", Name: "tool"}}}}
	if ShouldStop(ctx, StopAfterSteps(3), StopWhenNoToolCalls()) {
		t.Fatal("unexpected stop")
	}
	if !ShouldStop(ctx, StopAfterSteps(2), StopWhenNoToolCalls()) {
		t.Fatal("expected stop")
	}
}
