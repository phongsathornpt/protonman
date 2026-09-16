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
