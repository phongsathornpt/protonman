package common

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

func TestToneColorUsesSemanticTokens(t *testing.T) {
	tests := []struct {
		name string
		tone Tone
		want any
	}{
		{"assistant", ToneAssistant, tuistyle.ColorBorderFocus},
		{"user", ToneUser, tuistyle.AccentUser},
		{"warning", ToneWarning, tuistyle.ColorWarning},
		{"error", ToneError, tuistyle.ColorDanger},
	}
	for _, tt := range tests {
		if got := ToneColor(tt.tone); !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%s tone = %#v, want %#v", tt.name, got, tt.want)
		}
	}
}

func TestRenderToneModalFitsResponsiveWidths(t *testing.T) {
	for _, tc := range []struct {
		name   string
		width  int
		height int
	}{
		{"normal", 80, 24},
		{"compact", 32, 18},
		{"tiny", 20, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderToneModal(tc.width, tc.height, ToneWarning, []string{"Permission required", "Allow once"})
			if !strings.Contains(ansi.Strip(got), "Permission required") {
				t.Fatalf("modal missing content: %q", ansi.Strip(got))
			}
			for _, line := range strings.Split(got, "\n") {
				if width := ansi.StringWidth(line); width > tc.width {
					t.Fatalf("line width = %d, terminal width = %d: %q", width, tc.width, ansi.Strip(line))
				}
			}
		})
	}
}
