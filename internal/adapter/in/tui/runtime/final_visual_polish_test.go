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
	if got := ansi.StringWidth(frame.divider); got != 80 {
		t.Fatalf("divider width = %d, want terminal width 80", got)
	}
	if strings.Contains(ansi.Strip(frame.composer), "──") {
		t.Fatalf("composer retained nested border chrome: %q", ansi.Strip(frame.composer))
	}
	if got := strings.Count(ansi.Strip(frame.composer), "> "); got != 1 {
		t.Fatalf("composer prompt count = %d, want 1", got)
	}
}

func TestFinalVisualPolishHasTwoSectionRules(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.resize(80, 24)

	view := ansi.Strip(m.View().Content)
	rule := strings.Repeat("─", 80)
	if got := strings.Count(view, rule); got != 2 {
		t.Fatalf("section rule count = %d, want header + work rules; view=%q", got, view)
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
	status := strings.Index(view, "STATUS_SENTINEL")
	composer := strings.Index(view, "> ")
	if work < 0 || status < 0 || composer < 0 {
		t.Fatalf("final chrome markers missing: %q", view)
	}
	tail := view[work+len("WORK_SENTINEL"):]
	relativeDivider := strings.Index(tail, strings.Repeat("─", 8))
	if relativeDivider < 0 {
		t.Fatalf("work divider missing after transcript content: %q", view)
	}
	divider := work + len("WORK_SENTINEL") + relativeDivider
	if !(work < divider && divider < status && status < composer) {
		t.Fatalf("final chrome order changed: work=%d divider=%d status=%d composer=%d", work, divider, status, composer)
	}
}
