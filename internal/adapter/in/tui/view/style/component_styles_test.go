package style

import (
	"reflect"
	"testing"
)

func TestPaneComponentStylesUseSemanticTokens(t *testing.T) {
	if got := ActivityStyle.GetForeground(); !reflect.DeepEqual(got, ColorFocus) {
		t.Fatalf("activity foreground = %#v, want %#v", got, ColorFocus)
	}
	if !ActivityStyle.GetBold() {
		t.Fatal("activity style must be bold")
	}
	if got := PaneTitleStyle.GetForeground(); !reflect.DeepEqual(got, ColorTextPrimary) {
		t.Fatalf("pane title foreground = %#v, want %#v", got, ColorTextPrimary)
	}
	if !PaneTitleStyle.GetBold() {
		t.Fatal("pane title style must be bold")
	}
}
