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
