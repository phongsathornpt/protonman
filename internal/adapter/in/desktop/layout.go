//go:build desktop

package desktop

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

const auxiliarySectionsMaxHeight = 220

// desktopSurfaceLayout keeps the session header and composer fixed while
// giving expandable permission/runtime/MCP sections their own bounded scroll
// viewport. The conversation remains the primary flexible surface.
type desktopSurfaceLayout struct{}

func (desktopSurfaceLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 4 {
		return
	}
	gap := theme.Padding()
	header, auxiliary, footer, conversation := objects[0], objects[1], objects[2], objects[3]
	headerHeight := header.MinSize().Height
	footerHeight := footer.MinSize().Height
	auxiliaryHeight := float32(0)
	if auxiliary.Visible() {
		auxiliaryHeight = auxiliaryContentHeight(auxiliary)
	}
	available := size.Height - headerHeight - footerHeight - 3*gap
	if auxiliaryHeight > available {
		auxiliaryHeight = fyne.Max(0, available)
	}
	conversationHeight := fyne.Max(0, available-auxiliaryHeight)

	header.Move(fyne.NewPos(0, 0))
	header.Resize(fyne.NewSize(size.Width, headerHeight))
	y := headerHeight + gap
	auxiliary.Move(fyne.NewPos(0, y))
	auxiliary.Resize(fyne.NewSize(size.Width, auxiliaryHeight))
	y += auxiliaryHeight + gap
	conversation.Move(fyne.NewPos(0, y))
	conversation.Resize(fyne.NewSize(size.Width, conversationHeight))
	footer.Move(fyne.NewPos(0, size.Height-footerHeight))
	footer.Resize(fyne.NewSize(size.Width, footerHeight))
}

func (desktopSurfaceLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) < 4 {
		return fyne.NewSize(0, 0)
	}
	width := fyne.Max(objects[0].MinSize().Width, objects[1].MinSize().Width)
	width = fyne.Max(width, objects[2].MinSize().Width)
	width = fyne.Max(width, objects[3].MinSize().Width)
	height := objects[0].MinSize().Height + objects[2].MinSize().Height + objects[3].MinSize().Height + 3*theme.Padding()
	return fyne.NewSize(width, height)
}

func auxiliaryContentHeight(object fyne.CanvasObject) float32 {
	scroll, ok := object.(*container.Scroll)
	if !ok || scroll.Content == nil {
		return 0
	}
	height := scroll.Content.MinSize().Height
	if height > auxiliarySectionsMaxHeight {
		return auxiliarySectionsMaxHeight
	}
	return height
}

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

// minSizeLayout enforces minimum width and height on its children.
type minSizeLayout struct {
	width  float32
	height float32
}

func (l minSizeLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Move(fyne.NewPos(0, 0))
		object.Resize(size)
	}
}

func (l minSizeLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.width, l.height)
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
