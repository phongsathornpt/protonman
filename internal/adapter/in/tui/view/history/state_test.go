package history

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestScrollAnchorTracksCellAcrossEarlierExpansion(t *testing.T) {
	state := NewHistoryState(100)
	run := &AgentRunCell{AgentID: "worker-1", Profile: agent.ProfileStrength, Task: "fix", State: agent.StateRunning}
	target := &SystemCell{Text: "target"}
	state.Append(run)
	state.Append(target)
	anchor := state.CaptureScrollAnchor(2)
	run.Activity = "searching"
	state.TouchAgentRun("worker-1")
	got, ok := state.ResolveScrollAnchor(anchor)
	if !ok {
		t.Fatal("anchor was not resolved after earlier cell expansion")
	}
	if got != 3 {
		t.Fatalf("resolved line=%d, want 3", got)
	}
}
