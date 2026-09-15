package runtime

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestRuntimeCompactHeaderUsesResolvedIconProfile(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.icons = tuistyle.UnicodeIcons
	m.activeModel = "qwen3.8-27b"
	m.resize(32, 24)

	plain := ansi.Strip(m.sessionHeaderView())
	if !strings.Contains(plain, tuistyle.UnicodeIcons.Brand) {
		t.Fatalf("runtime compact header %q does not contain Unicode brand %q", plain, tuistyle.UnicodeIcons.Brand)
	}
}

func TestRuntimeASCIIHeaderUsesASCIIVisionGlyph(t *testing.T) {
	t.Setenv(envconfig.Icons, "ascii")
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "gemini-3.8-flash"
	m.resize(80, 24)

	plain := ansi.Strip(m.sessionHeaderView())
	if strings.Contains(plain, "👁") {
		t.Fatalf("ASCII header retained Unicode vision glyph: %q", plain)
	}
	if !strings.Contains(plain, "* vision") {
		t.Fatalf("ASCII header missing ASCII vision glyph: %q", plain)
	}
}
