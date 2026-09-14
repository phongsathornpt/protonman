package runtime

import (
	"testing"

	tuiicon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/icon"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestRuntimeIconModeResolvesFromEnvironment(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want tuiicon.Set
	}{
		{name: "auto", raw: "auto", want: tuiicon.Unicode},
		{name: "nerd", raw: "nerd", want: tuiicon.Nerd},
		{name: "unicode", raw: "unicode", want: tuiicon.Unicode},
		{name: "ascii", raw: "ascii", want: tuiicon.ASCII},
		{name: "invalid-safe-fallback", raw: "emoji-magic", want: tuiicon.Unicode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envconfig.Icons, tt.raw)
			m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
			if m.icons != tt.want {
				t.Fatalf("icons for %q = %#v, want %#v", tt.raw, m.icons, tt.want)
			}
		})
	}
}
