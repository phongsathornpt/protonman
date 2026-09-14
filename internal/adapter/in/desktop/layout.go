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

// responsiveDrawerLayout owns the conversation/drawer split. Unlike a Border
// child with a cached MinSize, this layout receives the current available width
// on every resize and recomputes the drawer allocation immediately.
type responsiveDrawerLayout struct{}

func (responsiveDrawerLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	content := objects[0]
	content.Move(fyne.NewPos(0, 0))
	if len(objects) < 2 || !objects[1].Visible() {
		content.Resize(size)
		if len(objects) > 1 {
			objects[1].Move(fyne.NewPos(size.Width, 0))
			objects[1].Resize(fyne.NewSize(0, size.Height))
		}
		return
	}

	drawer := objects[1]
	drawerWidth := contextDrawerWidthFor(size.Width)
	// Preserve the conversation as the primary surface on unusually narrow
	// windows. Normal desktop widths still use the 220..320px drawer policy.
	if maxWidth := size.Width * 0.45; drawerWidth > maxWidth {
		drawerWidth = maxWidth
	}
	if drawerWidth < 0 {
		drawerWidth = 0
	}
	contentWidth := size.Width - drawerWidth
	content.Resize(fyne.NewSize(contentWidth, size.Height))
	drawer.Move(fyne.NewPos(contentWidth, 0))
	drawer.Resize(fyne.NewSize(drawerWidth, size.Height))
}

func (responsiveDrawerLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 0)
}
