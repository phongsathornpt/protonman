package runtime

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestFinalVisualPolishUsesSingleWorkDivider(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.resize(80, 24)

	frame := m.layout.frame
	if frame.divider == "" {
		t.Fatal("composer region is missing its work divider")
	}
	if got, want := ansi.StringWidth(frame.divider), m.layoutProfile().ContentWidth(80); got != want {
		t.Fatalf("divider width = %d, want %d", got, want)
	}
	if strings.Contains(ansi.Strip(frame.composer), "──") {
		t.Fatalf("composer retained nested border chrome: %q", ansi.Strip(frame.composer))
	}
	if got := strings.Count(ansi.Strip(frame.composer), "> "); got != 1 {
		t.Fatalf("composer prompt count = %d, want 1", got)
	}
}

func TestFinalVisualPolishSharesContentWidth(t *testing.T) {
	for _, width := range []int{24, 40, 60, 80, 120} {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.runner = fakeConversation{}
		m.panes.bottom.setHasRunner(true)
		m.resize(width, 24)

		want := m.layoutProfile().ContentWidth(width)
		if got := m.viewport.Width(); got != want {
			t.Fatalf("viewport width at %d columns = %d, want %d", width, got, want)
		}
		if got := m.panes.bottom.prompt().Width(); got != want {
			t.Fatalf("composer width at %d columns = %d, want %d", width, got, want)
		}
	}
}

func TestFinalVisualPolishOrdersDividerBeforeLiveStatus(t *testing.T) {
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
	divider := strings.Index(view[work+len("WORK_SENTINEL"):], strings.Repeat("─", 8))
	status := strings.Index(view, "STATUS_SENTINEL")
	composer := strings.Index(view, "> ")
	if work < 0 || divider < 0 || status < 0 || composer < 0 {
		t.Fatalf("final chrome markers missing: %q", view)
	}
	divider += work + len("WORK_SENTINEL")
	if !(work < divider && divider < status && status < composer) {
		t.Fatalf("final chrome order changed: work=%d divider=%d status=%d composer=%d", work, divider, status, composer)
	}
}
