package tui

import "github.com/phongsathornpt/protonman/internal/adapter/in/tui/pane"

func normalizedPickerWindow(index, offset, count, visible int) (int, int, int) {
	return pane.NormalizedWindow(index, offset, count, visible)
}
