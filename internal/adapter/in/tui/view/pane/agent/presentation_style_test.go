package agent

import (
	"reflect"
	"testing"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	featureagent "github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestAgentStateStyleUsesLifecycleSemantics(t *testing.T) {
	tests := []struct {
		name  string
		state featureagent.State
		want  any
	}{
		{"running", featureagent.StateRunning, tuistyle.ColorFocus},
		{"resuming", featureagent.StateResuming, tuistyle.ColorFocus},
		{"queued", featureagent.StateQueued, tuistyle.ColorTextMuted},
		{"completed", featureagent.StateCompleted, tuistyle.ColorSuccess},
		{"failed", featureagent.StateFailed, tuistyle.ColorDanger},
		{"canceling", featureagent.StateCanceling, tuistyle.ColorWarning},
		{"interrupted", featureagent.StateInterrupted, tuistyle.ColorWarning},
		{"canceled", featureagent.StateCanceled, tuistyle.ColorTextMuted},
		{"resumed", featureagent.StateResumed, tuistyle.ColorTextMuted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agentStateStyle(tt.state).GetForeground(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("foreground = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestAgentPaneTitleUsesNeutralHierarchy(t *testing.T) {
	rows := AgentRows(AgentsSnapshot{Width: 80, Height: 24, SubagentsEnabled: true})
	if len(rows) == 0 || rows[0] != tuistyle.PaneTitleStyle.Render("Agents") {
		t.Fatalf("title row = %q", rows)
	}
}
