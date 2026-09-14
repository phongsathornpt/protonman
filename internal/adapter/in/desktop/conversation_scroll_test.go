//go:build desktop

package desktop

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	fynetest "fyne.io/fyne/v2/test"
)

func TestConversationTailFollowPreservesManualScroll(t *testing.T) {
	fynetest.NewTempApp(t)

	content := canvas.NewRectangle(color.Black)
	content.SetMinSize(fyne.NewSize(100, 1000))
	scroll := container.NewVScroll(content)
	scroll.Resize(fyne.NewSize(100, 200))
	a := &application{conversationScroll: scroll}

	scroll.Offset = fyne.NewPos(0, 800)
	if !a.shouldFollowConversationTail() {
		t.Fatal("expected tail position to follow streaming output")
	}

	scroll.Offset = fyne.NewPos(0, 300)
	if a.shouldFollowConversationTail() {
		t.Fatal("manual scroll away from tail should not be stolen")
	}
}

func TestConversationTailFollowAllowsSmallContent(t *testing.T) {
	fynetest.NewTempApp(t)

	content := canvas.NewRectangle(color.Black)
	content.SetMinSize(fyne.NewSize(100, 120))
	scroll := container.NewVScroll(content)
	scroll.Resize(fyne.NewSize(100, 200))
	a := &application{conversationScroll: scroll}

	if !a.shouldFollowConversationTail() {
		t.Fatal("content smaller than viewport should follow the tail")
	}
}
