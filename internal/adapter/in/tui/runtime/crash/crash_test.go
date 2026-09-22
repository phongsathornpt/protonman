package crash

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestCrashViewFitsTinyTerminals(t *testing.T) {
	model := NewCrashModel("a very long error message", []byte(strings.Repeat("stack trace line\n", 20)))
	for _, size := range [][2]int{{4, 4}, {12, 8}, {24, 10}} {
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		model = updated.(*CrashModel)
		view := model.View().Content
		if got := lipgloss.Height(view); got > size[1] {
			t.Fatalf("height=%d exceeds terminal height %d at %dx%d", got, size[1], size[0], size[1])
		}
		for _, line := range strings.Split(view, "\n") {
			if got := ansi.StringWidth(line); got > size[0] {
				t.Fatalf("line width=%d exceeds terminal width %d at %dx%d: %q", got, size[0], size[0], size[1], line)
			}
		}
	}
}

func TestCopyLabelStatesStayHonest(t *testing.T) {
	model := NewCrashModel("boom", []byte("stack"))
	cases := []struct {
		state CopyState
		want  string
	}{
		{CopyStatePending, "[c] Copy report"},
		{CopyStateConfirmed, "[✓ Copied to clipboard]"},
		{CopyStateTerminal, "[~ Sent to terminal"},
		{CopyStateFailed, "[!] Copy failed"},
	}
	for _, tc := range cases {
		model.copyState = tc.state
		view := model.View().Content
		if !strings.Contains(view, tc.want) {
			t.Fatalf("state %v view missing %q:\n%s", tc.state, tc.want, view)
		}
	}
	// Only a confirmed delivery may claim the system clipboard was written.
	model.copyState = CopyStateTerminal
	if got := model.View().Content; strings.Contains(got, "Copied to clipboard") {
		t.Fatalf("terminal-only copy claimed system clipboard:\n%s", got)
	}
	model.copyState = CopyStateFailed
	if got := model.View().Content; strings.Contains(got, "Copied to clipboard") || strings.Contains(got, "Sent to terminal") {
		t.Fatalf("failed copy claimed delivery:\n%s", got)
	}
}
