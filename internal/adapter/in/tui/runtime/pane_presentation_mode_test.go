package runtime

import "testing"

// paneOverlay is intentionally unreferenced in production code and kept only as
// the zero value. This test pins that fact so the constant is not deleted as
// dead code, which would silently change the zero-value behavior of a pane
// that forgets to implement PresentationMode.
func TestPaneOverlayRemainsTheZeroValue(t *testing.T) {
	var zero panePresentationMode
	if zero != paneOverlay {
		t.Fatalf("zero panePresentationMode = %d, want paneOverlay (%d)", zero, paneOverlay)
	}
	if paneBelowComposer == paneOverlay || paneBlocking == paneOverlay {
		t.Fatal("presentation modes must be distinct values")
	}
}
