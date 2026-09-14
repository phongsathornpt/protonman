//go:build desktop

package desktop

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

func TestFixedWidthLayoutBoundsInspectorMinSize(t *testing.T) {
	layout := fixedWidthLayout{width: 320}
	label := widget.NewLabel("a context value that would otherwise influence minimum size")
	label.Wrapping = fyne.TextWrapWord

	got := layout.MinSize([]fyne.CanvasObject{label})
	if got.Width != 320 {
		t.Fatalf("expected fixed width 320, got %v", got.Width)
	}
	if got.Height != 0 {
		t.Fatalf("expected inspector content not to force minimum height, got %v", got.Height)
	}
}

func TestResponsiveDrawerLayoutRecomputesWidthOnResize(t *testing.T) {
	content := canvas.NewRectangle(color.Black)
	drawer := canvas.NewRectangle(color.White)
	layout := responsiveDrawerLayout{}

	layout.Layout([]fyne.CanvasObject{content, drawer}, fyne.NewSize(1200, 700))
	if drawer.Size().Width != 320 {
		t.Fatalf("wide drawer width = %v, want 320", drawer.Size().Width)
	}
	if content.Size().Width != 880 {
		t.Fatalf("wide content width = %v, want 880", content.Size().Width)
	}

	layout.Layout([]fyne.CanvasObject{content, drawer}, fyne.NewSize(700, 700))
	if drawer.Size().Width != 220 {
		t.Fatalf("narrow drawer width = %v, want 220", drawer.Size().Width)
	}
	if content.Size().Width != 480 {
		t.Fatalf("narrow content width = %v, want 480", content.Size().Width)
	}
}

func TestResponsiveDrawerLayoutPreservesConversationOnVeryNarrowWidth(t *testing.T) {
	content := canvas.NewRectangle(color.Black)
	drawer := canvas.NewRectangle(color.White)
	layout := responsiveDrawerLayout{}

	layout.Layout([]fyne.CanvasObject{content, drawer}, fyne.NewSize(300, 500))
	if drawer.Size().Width != 135 {
		t.Fatalf("very narrow drawer width = %v, want 135", drawer.Size().Width)
	}
	if content.Size().Width != 165 {
		t.Fatalf("very narrow content width = %v, want 165", content.Size().Width)
	}

	drawer.Hide()
	layout.Layout([]fyne.CanvasObject{content, drawer}, fyne.NewSize(300, 500))
	if content.Size().Width != 300 {
		t.Fatalf("hidden drawer did not release width: %v", content.Size().Width)
	}
}
