package runtime

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestChromeContractResponsiveHierarchy(t *testing.T) {
	tests := []struct {
		name            string
		width           int
		height          int
		wantHeaderLines int
	}{
		{name: "wide", width: 80, height: 24, wantHeaderLines: 5},
		{name: "compact", width: 32, height: 24, wantHeaderLines: 3},
		{name: "tiny", width: 20, height: 12, wantHeaderLines: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
			m.runner = fakeConversation{}
			m.panes.bottom.setHasRunner(true)
			m.activeModel = "qwen3.8-27b"
			m.resize(tt.width, tt.height)

			frame := m.layout.frame
			if frame.header == "" {
				t.Fatal("responsive chrome unexpectedly hid the session header")
			}
			if got := strings.Count(frame.header, "\n") + 1; got != tt.wantHeaderLines {
				t.Fatalf("header rows = %d, want %d: %q", got, tt.wantHeaderLines, ansi.Strip(frame.header))
			}
			if frame.status != "" {
				t.Fatalf("idle chrome rendered a status row: %q", ansi.Strip(frame.status))
			}
			if frame.composer == "" {
				t.Fatal("idle chrome lost the composer")
			}
			footer := ansi.Strip(frame.footer)
			if !strings.Contains(footer, "?") || !strings.Contains(footer, "ask") {
				t.Fatalf("idle footer lost primary context: %q", footer)
			}
			assertRenderedFrameFits(t, m.View().Content, tt.width, tt.height)
		})
	}
}

func TestChromeContractBusyStatusUsesStableRegion(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.activeModel = "qwen3.8-27b"
	m.reducedMotion = true
	m.resize(80, 24)

	idleViewportHeight := m.viewport.Height()
	m.busy = true
	m.activity = "running tests"
	m.requestRelayout()
	m.reconcileLayout()

	status := ansi.Strip(m.layout.frame.status)
	if status != "◌ running tests" {
		t.Fatalf("busy status = %q, want %q", status, "◌ running tests")
	}
	if got := m.viewport.Height(); got != idleViewportHeight-1 {
		t.Fatalf("busy viewport height = %d, want idle %d minus one status row", got, idleViewportHeight)
	}
	if footer := ansi.Strip(m.layout.frame.footer); !strings.Contains(footer, "?") || !strings.Contains(footer, "ask") {
		t.Fatalf("busy chrome changed stable footer context: %q", footer)
	}
	assertRenderedFrameFits(t, m.View().Content, 80, 24)
}

func TestChromeContractOrdersIdentityWorkStatusInputContext(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.activeModel = "qwen3.8-27b"
	m.reducedMotion = true
	m.resize(80, 24)
	m.appendLine("WORK_SENTINEL")
	m.refreshViewport()
	m.busy = true
	m.activity = "STATUS_SENTINEL"
	m.requestRelayout()
	m.reconcileLayout()

	view := ansi.Strip(m.View().Content)
	identity := strings.Index(view, "protonMAN")
	work := strings.Index(view, "WORK_SENTINEL")
	status := strings.Index(view, "STATUS_SENTINEL")
	composer := strings.Index(view, "> ")
	footer := strings.Index(view, "? for shortcuts")
	positions := []struct {
		name string
		pos  int
	}{{"identity", identity}, {"work", work}, {"status", status}, {"composer", composer}, {"footer", footer}}
	for _, item := range positions {
		if item.pos < 0 {
			t.Fatalf("%s marker missing from composed view: %q", item.name, view)
		}
	}
	if !(identity < work && work < status && status < composer && composer < footer) {
		t.Fatalf("chrome hierarchy changed: identity=%d work=%d status=%d composer=%d footer=%d", identity, work, status, composer, footer)
	}
}

func TestChromeContractComposerAppearsExactlyOnce(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {72, 20}, {80, 24}, {120, 32}} {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.runner = fakeConversation{}
		m.panes.bottom.setHasRunner(true)
		m.resize(size[0], size[1])
		view := ansi.Strip(m.View().Content)
		if got := strings.Count(view, "> "); got != 1 {
			t.Fatalf("composer count at %dx%d = %d, want 1", size[0], size[1], got)
		}
	}
}
