package agent

import (
	"strings"
	"testing"
	"time"

	featureagent "github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestAgentRowsShowsDependencyLineage(t *testing.T) {
	rows := AgentRows(AgentsSnapshot{
		Width:            80,
		Height:           30,
		SubagentsEnabled: true,
		Now:              time.Now(),
		Retained: []featureagent.AgentStatus{{
			ID: "strength-3", Profile: featureagent.ProfileStrength, Task: "apply fix",
			DependsOn: []string{"agility-1", "intelligence-2"}, State: featureagent.StateQueued,
			StartTime: time.Now().Add(-time.Second),
		}},
	})
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"W8", "deps · agility-1, intelligence-2", "apply fix"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("rows missing %q: %s", want, joined)
		}
	}
}

func TestAgentRowsShowsIntegratedCompletedResult(t *testing.T) {
	now := time.Now()
	rows := AgentRows(AgentsSnapshot{
		Width: 80, Height: 30, SubagentsEnabled: true, Now: now,
		Retained: []featureagent.AgentStatus{{
			ID: "agility-1", Profile: featureagent.ProfileAgility, Task: "inspect flow",
			State: featureagent.StateCompleted, StartTime: now.Add(-time.Second),
			StartedAt: now.Add(-time.Second), FinishedAt: now,
		}},
		Activity: map[string]string{"agility-1": "Integrated"},
	})
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "Integrated") {
		t.Fatalf("rows missing integrated result state: %s", joined)
	}
}
