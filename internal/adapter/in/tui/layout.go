package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type terminalLayoutMode uint8

const (
	layoutNormal terminalLayoutMode = iota
	layoutCompact
	layoutTiny
)

func layoutModeForHeight(height int) terminalLayoutMode {
	switch {
	case height < 14:
		return layoutTiny
	case height < 20:
		return layoutCompact
	default:
		return layoutNormal
	}
}

func pickerVisibleRows(height, maximum int) int {
	rows := maximum
	switch {
	case height <= 12:
		rows = 2
	case height <= 14:
		rows = 3
	case height <= 20:
		rows = 4
	}
	if rows < 1 {
		return 1
	}
	if maximum > 0 && rows > maximum {
		return maximum
	}
	return rows
}

func renderModalRows(m *bubbleModel, border lipgloss.TerminalColor, rows []string) string {
	style := modalStyle.
		BorderForeground(border).
		MaxWidth(maxInt(1, m.width-4))
	if layoutModeForHeight(m.height) != layoutNormal {
		style = style.Padding(0, 1)
	}
	return style.Render(strings.Join(rows, "\n"))
}

func compactPickerRows(rows []string) []string {
	compact := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row) == "" {
			continue
		}
		compact = append(compact, row)
	}
	return compact
}
