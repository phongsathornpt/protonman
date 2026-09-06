package tui

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
