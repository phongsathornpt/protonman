package history

import (
	"reflect"
	"testing"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestAgentRunCellStatePresentationUsesSemanticLifecycleColors(t *testing.T) {
	tests := []struct {
		name  string
		state agent.State
		want  any
	}{
		{name: "running", state: agent.StateRunning, want: tuistyle.FocusStyle.GetForeground()},
		{name: "queued", state: agent.StateQueued, want: tuistyle.MutedStyle.GetForeground()},
		{name: "completed", state: agent.StateCompleted, want: tuistyle.SuccessStyle.GetForeground()},
		{name: "failed", state: agent.StateFailed, want: tuistyle.ErrorStyle.GetForeground()},
		{name: "canceled", state: agent.StateCanceled, want: tuistyle.MutedStyle.GetForeground()},
		{name: "interrupted", state: agent.StateInterrupted, want: tuistyle.WarningStyle.GetForeground()},
		{name: "canceling", state: agent.StateCanceling, want: tuistyle.WarningStyle.GetForeground()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, gotStyle := (AgentRunCell{State: tt.state}).statePresentation()
			if got := gotStyle.GetForeground(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("foreground = %#v, want %#v", got, tt.want)
			}
		})
	}
}
