//go:build desktop

package desktop

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type nerdIcon string

const (
	iconRocket     nerdIcon = "\uf135"
	iconFolder     nerdIcon = "\uf07b"
	iconSession    nerdIcon = "\uf075"
	iconReady      nerdIcon = "\uf111"
	iconQueued     nerdIcon = "\uf017"
	iconRunning    nerdIcon = "\uf110"
	iconPermission nerdIcon = "\uf084"
	iconWaiting    nerdIcon = "\uf254"
	iconPaused     nerdIcon = "\uf04c"
	iconCompleted  nerdIcon = "\uf058"
	iconFailed     nerdIcon = "\uf057"
)

func newNerdIconText(icon nerdIcon, text string, style fyne.TextStyle, lowImportance bool) *fyne.Container {
	glyph := canvas.NewText(string(icon), theme.ForegroundColor())
	glyph.TextSize = theme.TextSize()
	glyph.TextStyle = fyne.TextStyle{Symbol: true}
	glyph.FontSource = loadNerdFontResource()
	label := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, style)
	if lowImportance {
		label.Importance = widget.LowImportance
	}
	return container.NewHBox(glyph, label)
}

func setNerdIconText(row *fyne.Container, icon nerdIcon, text string) {
	if row == nil || len(row.Objects) < 2 {
		return
	}
	glyph, glyphOK := row.Objects[0].(*canvas.Text)
	label, labelOK := row.Objects[1].(*widget.Label)
	if !glyphOK || !labelOK {
		return
	}
	glyph.Text = string(icon)
	canvas.Refresh(glyph)
	label.SetText(text)
}

func taskStatusIcon(status desktopstate.TaskStatus) nerdIcon {
	switch status {
	case desktopstate.TaskQueued:
		return iconQueued
	case desktopstate.TaskRunning:
		return iconRunning
	case desktopstate.TaskWaitingPermission:
		return iconPermission
	case desktopstate.TaskWaitingUser:
		return iconWaiting
	case desktopstate.TaskPaused:
		return iconPaused
	case desktopstate.TaskCompleted:
		return iconCompleted
	case desktopstate.TaskFailed:
		return iconFailed
	default:
		return iconReady
	}
}
