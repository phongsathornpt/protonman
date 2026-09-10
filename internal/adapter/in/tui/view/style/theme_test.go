package style

import (
	"reflect"
	"testing"
)

func TestSemanticStylesUseThemePalette(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"assistant", AssistantStyle.GetForeground(), ColorTextPrimary},
		{"body", BodyStyle.GetForeground(), ColorTextPrimary},
		{"tool target", ToolTargetStyle.GetForeground(), ColorTextSecondary},
		{"diff hunk", DiffHunkStyle.GetForeground(), ColorTextSecondary},
		{"markdown heading", MarkdownHeadingStyle.GetForeground(), ColorTextPrimary},
		{"markdown bullet", MarkdownBulletStyle.GetForeground(), ColorTextSecondary},
	}
	for _, tt := range tests {
		if !reflect.DeepEqual(tt.got, tt.want) {
			t.Errorf("%s foreground = %#v, want %#v", tt.name, tt.got, tt.want)
		}
	}
}
