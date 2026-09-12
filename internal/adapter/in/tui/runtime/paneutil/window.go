package paneutil

import panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Window computes the visible start and end slice indices for a paginated list.
func Window(count, selected, maximum int, mode panecommon.LayoutMode) (int, int) {
	visible := maximum
	switch mode {
	case panecommon.LayoutTiny:
		visible = minInt(2, maximum)
	case panecommon.LayoutCompact:
		visible = minInt(4, maximum)
	}
	if visible < 1 {
		visible = 1
	}
	if count <= visible {
		return 0, count
	}
	start := selected - visible + 1
	if start < 0 {
		start = 0
	}
	if start+visible > count {
		start = count - visible
	}
	return start, start + visible
}
