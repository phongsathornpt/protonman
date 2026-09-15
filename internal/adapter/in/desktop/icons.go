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

type icon string

const (
	iconRocket     icon = "◆"
	iconFolder     icon = "▸"
	iconSession    icon = "●"
	iconReady      icon = "○"
	iconQueued     icon = "◌"
	iconRunning    icon = "◐"
	iconPermission icon = "!"
	iconWaiting    icon = "…"
	iconPaused     icon = "Ⅱ"
	iconCompleted  icon = "✓"
	iconFailed     icon = "×"
)

func newIconText(icon icon, text string, style fyne.TextStyle, lowImportance bool) *fyne.Container {
	glyph := canvas.NewText(string(icon), theme.ForegroundColor())
	glyph.TextSize = theme.TextSize()
	label := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, style)
	if lowImportance {
		label.Importance = widget.LowImportance
	}
	return container.NewHBox(glyph, label)
}

func setIconText(row *fyne.Container, icon icon, text string) {
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

func taskStatusIcon(status desktopstate.TaskStatus) icon {
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
