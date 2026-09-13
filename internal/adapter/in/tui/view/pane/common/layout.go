// Package pane contains presentation-only TUI pane renderers and layout policy.
package common

import (
	"image/color"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

const (
	wideLayoutMinWidth     = 40
	compactLayoutMinWidth  = 24
	headerVisibleMinHeight = 10
	overlayHeaderMinHeight = 18
	defaultHorizontalInset = 1
	minimumViewportHeight  = 1
)

type Tone uint8

const (
	ToneAssistant Tone = iota
	ToneUser
	ToneError
	ToneWarning
)

// LayoutMode describes terminal-space constraints for TUI presentation.
type LayoutMode uint8

const (
	LayoutNormal LayoutMode = iota
	LayoutCompact
	LayoutTiny
)

// ModeForSize is the single responsive breakpoint policy for terminal layout.
// Width can downgrade a vertically roomy terminal so panes and chrome make the
// same compactness decision as the top-level frame.
func ModeForSize(width, height int) LayoutMode {
	switch {
	case width < compactLayoutMinWidth || height < 14:
		return LayoutTiny
	case width < wideLayoutMinWidth || height < 20:
		return LayoutCompact
	default:
		return LayoutNormal
	}
}

// ModeForHeight preserves the height-only pane contract while sharing the same
// breakpoint implementation as the top-level frame.
func ModeForHeight(height int) LayoutMode {
	return ModeForSize(wideLayoutMinWidth, height)
}

// Profile describes shared top-level layout decisions derived from terminal
// size and whether a transient bottom surface is active.
type Profile struct {
	Mode            LayoutMode
	ShowHeader      bool
	HorizontalInset int
}

func ResolveProfile(width, height int, hasBottomView bool) Profile {
	profile := Profile{
		Mode:            ModeForSize(width, height),
		ShowHeader:      true,
		HorizontalInset: defaultHorizontalInset,
	}
	if height < headerVisibleMinHeight || (hasBottomView && height < overlayHeaderMinHeight) {
		profile.ShowHeader = false
	}
	return profile
}

func (p Profile) ContentWidth(terminalWidth int) int {
	width := terminalWidth - (p.HorizontalInset * 2)
	if width < 1 {
		return 1
	}
	return width
}

func (p Profile) CompactHeader() bool {
	return p.Mode != LayoutNormal
}

func (p Profile) MinimalHeader() bool {
	return p.Mode == LayoutTiny
}

type FrameGeometry struct {
	TerminalWidth  int
	TerminalHeight int
	ChromeHeight   int
	ViewportHeight int
}

func ResolveFrameGeometry(width, height, chromeHeight int) FrameGeometry {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if chromeHeight < 0 {
		chromeHeight = 0
	}
	viewportHeight := height - chromeHeight
	if viewportHeight < minimumViewportHeight {
		viewportHeight = minimumViewportHeight
	}
	return FrameGeometry{
		TerminalWidth:  width,
		TerminalHeight: height,
		ChromeHeight:   chromeHeight,
		ViewportHeight: viewportHeight,
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
