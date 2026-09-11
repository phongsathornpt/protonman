package history

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestAgentRunCellInterruptedIsRecoverableWarning(t *testing.T) {
	cell := AgentRunCell{Profile: agent.ProfileStrength, Task: "continue refactor", State: agent.StateInterrupted, Reason: "interrupted by previous process exit"}
	got := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(got, "continue refactor") || !strings.Contains(got, "interrupted by previous process exit") {
		t.Fatalf("render = %q", got)
	}
	fallback := AgentRunCell{Profile: agent.ProfileStrength, Task: "continue refactor", State: agent.StateInterrupted}
	if got := strings.Join(fallback.RenderWidth(80), "\n"); !strings.Contains(got, "interrupted · resume available") {
		t.Fatalf("fallback render = %q", got)
	}
}

func TestAgentRunCellCompletedShowsIntegratedActivity(t *testing.T) {
	cell := AgentRunCell{Profile: agent.ProfileAgility, Task: "inspect flow", State: agent.StateCompleted, Activity: "Integrated"}
	got := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(got, "Integrated") {
		t.Fatalf("render = %q", got)
	}
}
