package runtime

import (
	"testing"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestRuntimeIconModeResolvesFromEnvironment(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want tuistyle.IconSet
	}{
		{name: "auto", raw: "auto", want: tuistyle.UnicodeIcons},
		{name: "unsupported-nerd", raw: "nerd", want: tuistyle.UnicodeIcons},
		{name: "unicode", raw: "unicode", want: tuistyle.UnicodeIcons},
		{name: "ascii", raw: "ascii", want: tuistyle.ASCIIIcons},
		{name: "invalid-safe-fallback", raw: "emoji-magic", want: tuistyle.UnicodeIcons},
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
