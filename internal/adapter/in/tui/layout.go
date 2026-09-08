package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/pane"
)

type terminalLayoutMode = pane.LayoutMode

const (
	layoutNormal  = pane.LayoutNormal
	layoutCompact = pane.LayoutCompact
	layoutTiny    = pane.LayoutTiny
)

func layoutModeForHeight(height int) terminalLayoutMode { return pane.ModeForHeight(height) }
func pickerVisibleRows(height, maximum int) int         { return pane.PickerVisibleRows(height, maximum) }
func compactPickerRows(rows []string) []string          { return pane.CompactRows(rows) }

func renderModalRows(m *bubbleModel, border lipgloss.TerminalColor, rows []string) string {
	if m == nil {
		return pane.RenderModal(defaultBubbleWidth, defaultBubbleHeight, border, rows)
	}
	return pane.RenderModal(m.width, m.height, border, rows)
}
