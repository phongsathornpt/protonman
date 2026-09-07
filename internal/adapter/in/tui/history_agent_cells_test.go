package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/feature/agent"
)

func TestAgentRunCellKeepsTaskAndFailureReason(t *testing.T) {
	started := time.Unix(100, 0)
	cell := AgentRunCell{
		AgentID: "dex-7", Profile: agent.ProfileDEX,
		Task: "review concurrency", State: agent.StateFailed,
		Reason: "timed out", StartedAt: started, FinishedAt: started.Add(30 * time.Second),
	}
	got := strings.Join(cell.RenderWidth(80), "\n")
	for _, want := range []string{"DEX review concurrency", "timed out", "30.0s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("render=%q, want %q", got, want)
		}
	}
}

func TestAgentRunCellCompletedShowsSummary(t *testing.T) {
	started := time.Unix(200, 0)
	cell := AgentRunCell{
		AgentID: "int-2", Profile: agent.ProfileINT,
		Task: "inspect router", State: agent.StateCompleted,
		Summary: "found routing boundary", StartedAt: started, FinishedAt: started.Add(8*time.Second + 400*time.Millisecond),
	}
	got := strings.Join(cell.RenderWidth(80), "\n")
	for _, want := range []string{"INT inspect router", "found routing boundary", "8.4s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("render=%q, want %q", got, want)
		}
	}
}

func TestAgentRunCellRunningUsesActivityWithoutFakeDuration(t *testing.T) {
	cell := AgentRunCell{
		AgentID: "int-3", Profile: agent.ProfileINT,
		Task: "trace cache", State: agent.StateRunning,
		Activity: `Search "routeRequest"`, StartedAt: time.Unix(300, 0), Spinner: "⠋",
	}
	got := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(got, "trace cache") || !strings.Contains(got, `Search "routeRequest"`) {
		t.Fatalf("render=%q", got)
	}
	if strings.Contains(got, "0s") {
		t.Fatalf("running cell exposed fake duration: %q", got)
	}
}
