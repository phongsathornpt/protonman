// Package pane contains presentation-only TUI pane renderers and layout policy.
package common

import (
	"image/color"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

type Tone uint8

const (
	ToneAssistant Tone = iota
	ToneUser
	ToneError
	ToneWarning
)

// LayoutMode describes vertical-space constraints for transient panes.
type LayoutMode uint8

const (
	LayoutNormal LayoutMode = iota
	LayoutCompact
	LayoutTiny
)

func ModeForHeight(height int) LayoutMode {
	switch {
	case height < 14:
		return LayoutTiny
	case height < 20:
		return LayoutCompact
	default:
		return LayoutNormal
	}
}

func PickerVisibleRows(height, maximum int) int {
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

func CompactRows(rows []string) []string {
	compact := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row) != "" {
			compact = append(compact, row)
		}
	}
	return compact
}

func RenderModal(width, height int, border color.Color, rows []string) string {
	style := tuistyle.ModalStyle.BorderForeground(border).MaxWidth(max(1, width-4))
	if ModeForHeight(height) != LayoutNormal {
		style = style.Padding(0, 1)
	}
	return style.Render(strings.Join(rows, "\n"))
}

func NormalizedWindow(index, offset, count, visible int) (int, int, int) {
	if count <= 0 {
		return 0, 0, 0
	}
	if visible <= 0 {
		visible = 1
	}
	if index < 0 {
		index = 0
	} else if index >= count {
		index = count - 1
	}
	if offset < 0 {
		offset = 0
	}
	if index < offset {
		offset = index
	} else if index >= offset+visible {
		offset = index - visible + 1
	}
	maxOffset := max(0, count-visible)
	if offset > maxOffset {
		offset = maxOffset
	}
	end := min(count, offset+visible)
	return index, offset, end
}
