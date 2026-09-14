//go:build desktop

package desktop

import "fyne.io/fyne/v2"

// fixedWidthLayout constrains inspector-like side surfaces so their content
// cannot grow the whole window's minimum width. The child is still laid out
// at the space assigned by the parent, so wrapped text remains responsive.
type fixedWidthLayout struct {
	width float32
}

func (l fixedWidthLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Move(fyne.NewPos(0, 0))
		object.Resize(size)
	}
}

func (l fixedWidthLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	height := float32(0)
	for _, object := range objects {
		if min := object.MinSize(); min.Height > height {
			height = min.Height
		}
	}
	return fyne.NewSize(l.width, height)
}
