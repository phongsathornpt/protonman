package common

// PaneContentWidth returns the interior content width for a pane whose outer
// width is width: max(1, width-8). The 8-cell inset source is the pane border
// plus horizontal padding, so this is the truncation/set width for content
// rows, list items, and pickers drawn inside the pane.
func PaneContentWidth(width int) int {
	return max(1, width-8)
}

// PaneHelpWidth returns the row width for the trailing help/status row of a
// pane whose outer width is width: max(1, width-6). The 6-cell inset source is
// the border-and-padding budget reserved for help and status rows inside a pane.
func PaneHelpWidth(width int) int {
	return max(1, width-6)
}
