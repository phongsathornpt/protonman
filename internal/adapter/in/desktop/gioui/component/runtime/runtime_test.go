//go:build desktop || desktop_gio

package runtime

import "testing"

func TestClosePopoversClosesEveryRuntimeSurface(t *testing.T) {
	component := New()
	widgets := component.Widgets()
	widgets.ModelPopoverVisible = true
	widgets.ReasoningPopoverVisible = true
	widgets.PermissionModePopoverVisible = true

	component.ClosePopovers()

	if widgets.ModelPopoverVisible || widgets.ReasoningPopoverVisible || widgets.PermissionModePopoverVisible {
		t.Fatal("ClosePopovers left a runtime surface visible")
	}
}
