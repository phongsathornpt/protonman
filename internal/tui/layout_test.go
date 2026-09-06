package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestPickerVisibleRows(t *testing.T) {
	tests := []struct {
		height  int
		maximum int
		want    int
	}{
		{height: 12, maximum: 6, want: 2},
		{height: 14, maximum: 6, want: 3},
		{height: 18, maximum: 6, want: 4},
		{height: 24, maximum: 6, want: 6},
		{height: 24, maximum: 5, want: 5},
	}
	for _, tc := range tests {
		if got := pickerVisibleRows(tc.height, tc.maximum); got != tc.want {
			t.Fatalf("pickerVisibleRows(%d, %d) = %d, want %d", tc.height, tc.maximum, got, tc.want)
		}
	}
}

func TestLayoutModeBreakpoints(t *testing.T) {
	if got := layoutModeForHeight(24); got != layoutNormal {
		t.Fatalf("24 rows mode = %v", got)
	}
	if got := layoutModeForHeight(18); got != layoutCompact {
		t.Fatalf("18 rows mode = %v", got)
	}
	if got := layoutModeForHeight(12); got != layoutTiny {
		t.Fatalf("12 rows mode = %v", got)
	}
}

func TestPickersFitResponsiveTerminalHeights(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 18}, {40, 14}, {24, 12}} {
		m := newTestSkillsModel(t, 10)
		m.resize(size[0], size[1])

		modelView := newModelSelectPaneView(m).Render(m)
		if got := lipgloss.Height(modelView); got > size[1] {
			t.Fatalf("model picker height %d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		if got := lipgloss.Width(modelView); got > size[0] {
			t.Fatalf("model picker width %d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}

		skillsView := (&skillsPaneView{}).Render(m)
		if got := lipgloss.Height(skillsView); got > size[1] {
			t.Fatalf("skills picker height %d exceeds %d at %dx%d", got, size[1], size[0], size[1])
		}
		if got := lipgloss.Width(skillsView); got > size[0] {
			t.Fatalf("skills picker width %d exceeds %d at %dx%d", got, size[0], size[0], size[1])
		}
	}
}
