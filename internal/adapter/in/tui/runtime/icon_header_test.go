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
	m.icons = tuistyle.NerdIcons
	m.activeModel = "qwen3.8-27b"
	m.resize(32, 24)

	plain := ansi.Strip(m.sessionHeaderView())
	if !strings.Contains(plain, tuistyle.NerdIcons.Brand) {
		t.Fatalf("runtime compact header %q does not contain Nerd brand %q", plain, tuistyle.NerdIcons.Brand)
	}
}
