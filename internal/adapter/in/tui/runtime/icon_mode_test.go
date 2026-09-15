package runtime

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"github.com/charmbracelet/x/ansi"
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

func TestRuntimeASCIIUsesASCIIBusyIndicator(t *testing.T) {
	t.Setenv(envconfig.Icons, "ascii")
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.reducedMotion = true
	m.busy = true
	m.activity = "working"

	if got := ansi.Strip(m.statusView()); !strings.HasPrefix(got, "o ") {
		t.Fatalf("ASCII busy status = %q, want ASCII indicator prefix", got)
	}
	if len(m.spinner.Spinner.Frames) == 0 || m.spinner.Spinner.Frames[0] != spinner.Line.Frames[0] {
		t.Fatalf("ASCII spinner = %#v, want line spinner", m.spinner.Spinner)
	}
}
