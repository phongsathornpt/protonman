package runtime

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestFinalVisualPolishUsesEnclosedComposerCard(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.resize(80, 24)

	frame := m.layout.frame
	composer := ansi.Strip(frame.composer)
	if !strings.HasPrefix(composer, "╭") {
		t.Fatalf("composer card missing rounded top-left corner: %q", composer)
	}
	if !strings.HasSuffix(composer, "╯") {
		t.Fatalf("composer card missing rounded bottom-right corner: %q", composer)
	}
	if got := strings.Count(composer, "> "); got != 1 {
		t.Fatalf("composer prompt count = %d, want 1", got)
	}
	lines := strings.Split(composer, "\n")
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != 80 {
			t.Fatalf("line %d width = %d, want 80: %q", i, got, line)
		}
	}
}

func TestFinalVisualPolishHasCardAndSectionChrome(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.resize(80, 24)

	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "╭") || !strings.Contains(view, "╰") {
		t.Fatalf("view missing enclosed card corners; view=%q", view)
	}
	rule := strings.Repeat("─", 80)
	if !strings.Contains(view, rule) {
		t.Fatalf("view missing header section rule; view=%q", view)
	}
}

func TestFinalVisualPolishOrdersStatusBeforeComposer(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.reducedMotion = true
	m.resize(80, 24)
	m.appendLine("WORK_SENTINEL")
	m.refreshViewport()
	m.busy = true
	m.activity = "STATUS_SENTINEL"
	m.requestRelayout()
	m.reconcileLayout()

	view := ansi.Strip(m.View().Content)
	work := strings.Index(view, "WORK_SENTINEL")
	status := strings.Index(view, "STATUS_SENTINEL")
	composer := strings.Index(view, "╭")
	if work < 0 || status < 0 || composer < 0 {
		t.Fatalf("final chrome markers missing: %q", view)
	}
	if !(work < status && status < composer) {
		t.Fatalf("final chrome order changed: work=%d status=%d composer=%d", work, status, composer)
	}
}
