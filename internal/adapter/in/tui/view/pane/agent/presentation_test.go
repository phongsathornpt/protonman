package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
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

func TestAgentRowsTruncatesOverlongIdentifier(t *testing.T) {
	longID := strings.Repeat("agility-", 20)
	rows := AgentRows(AgentsSnapshot{
		Width: 40, Height: 30, SubagentsEnabled: true, Now: time.Now(),
		Retained: []featureagent.AgentStatus{{
			ID: longID, Profile: featureagent.ProfileAgility, Task: "inspect",
			State: featureagent.StateRunning, StartTime: time.Now(),
		}},
	})
	for _, row := range rows {
		if width := ansi.StringWidth(row); width > 40 {
			t.Fatalf("agent row exceeded snapshot width (%d): %q", width, row)
		}
		if strings.Contains(row, longID) {
			t.Fatalf("agent identifier was not truncated: %q", row)
		}
	}
}

func TestAgentRowsIncludesStatusLegend(t *testing.T) {
	rows := AgentRows(AgentsSnapshot{
		Width: 80, Height: 30, SubagentsEnabled: true, Now: time.Now(),
		Retained: []featureagent.AgentStatus{{
			ID: "agility-1", Profile: featureagent.ProfileAgility, Task: "inspect",
			State: featureagent.StateRunning, StartTime: time.Now(),
		}},
	})
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "Status: W8 queued") {
		t.Fatalf("expected legend in normal height pane, got: %s", joined)
	}
}

