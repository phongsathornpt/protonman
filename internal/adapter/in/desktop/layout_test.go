//go:build desktop

package desktop

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

func TestFixedWidthLayoutBoundsInspectorWidth(t *testing.T) {
	layout := fixedWidthLayout{width: 320}
	label := widget.NewLabel("a context value that would otherwise influence minimum width")
	label.Wrapping = fyne.TextWrapWord

	got := layout.MinSize([]fyne.CanvasObject{label})
	if got.Width != 320 {
		t.Fatalf("expected fixed width 320, got %v", got.Width)
	}
	if got.Height <= 0 {
		t.Fatalf("expected positive minimum height, got %v", got.Height)
	}
}
