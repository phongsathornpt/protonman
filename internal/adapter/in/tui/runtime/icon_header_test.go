package runtime

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
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
