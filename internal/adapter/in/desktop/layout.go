//go:build desktop

package desktop

import "fyne.io/fyne/v2"

// fixedWidthLayout constrains inspector-like side surfaces so their content
// cannot grow the window's minimum width or height. The child is always resized
// to the space assigned by the parent, allowing scroll containers and wrapped
// text to adapt without forcing the outer window larger.
type fixedWidthLayout struct {
	width float32
}

func (l fixedWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Move(fyne.NewPos(0, 0))
		object.Resize(size)
	}
}

func (l fixedWidthLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.width, 0)
}
