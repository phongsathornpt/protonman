package runtime

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestRuntimeLayoutCanonicalTerminalInvariants(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {72, 20}, {80, 24}, {100, 30}, {120, 32}, {160, 50}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
			m.runner = fakeConversation{}
			m.panes.bottom.setHasRunner(true)
			m.activeModel = "qwen3.8-27b"
			m.resize(size[0], size[1])

			geometry := m.layout.geometry
			if geometry.TerminalWidth != size[0] || geometry.TerminalHeight != size[1] {
				t.Fatalf("geometry terminal = %dx%d, want %dx%d", geometry.TerminalWidth, geometry.TerminalHeight, size[0], size[1])
			}
			if geometry.ViewportHeight < 1 {
				t.Fatalf("viewport height = %d, want >= 1", geometry.ViewportHeight)
			}
			if m.viewport.Height() != geometry.ViewportHeight {
				t.Fatalf("viewport model height = %d, geometry = %d", m.viewport.Height(), geometry.ViewportHeight)
			}
			if m.layout.frame.height < size[1] && m.viewport.Height()+m.layout.frame.height != size[1] {
				t.Fatalf("viewport + chrome = %d + %d = %d, want %d", m.viewport.Height(), m.layout.frame.height, m.viewport.Height()+m.layout.frame.height, size[1])
			}

			assertRenderedFrameFits(t, m.View().Content, size[0], size[1])
		})
	}
}

func TestRuntimeBusyLayoutFitsCanonicalTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{60, 16}, {72, 20}, {80, 24}, {100, 30}, {120, 32}, {160, 50}} {
		m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
		m.runner = fakeConversation{}
		m.panes.bottom.setHasRunner(true)
		m.activeModel = "qwen3.8-27b"
		m.resize(size[0], size[1])
		m.busy = true
		m.activity = strings.Repeat("進捗ภาษาไทย", 12)
		m.busyStarted = time.Now().Add(-3 * time.Second)
		m.requestRelayout()
		m.reconcileLayout()

		if m.layout.frame.status == "" {
			t.Fatalf("busy frame at %dx%d has no status row", size[0], size[1])
		}
		if width := ansi.StringWidth(m.layout.frame.status); width > m.layoutProfile().ContentWidth(size[0]) {
			t.Fatalf("status width = %d exceeds content width at %dx%d: %q", width, size[0], size[1], ansi.Strip(m.layout.frame.status))
		}
		assertRenderedFrameFits(t, m.View().Content, size[0], size[1])
	}
}

func TestRuntimeBottomViewHeaderVisibilityBoundary(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "qwen3.8-27b"
	m.panes.bottom.push(&todoPaneView{})

	m.resize(80, 17)
	if got := m.sessionHeaderView(); got != "" {
		t.Fatalf("header visible with bottom view at 17 rows: %q", ansi.Strip(got))
	}

	m.resize(80, 18)
	if got := m.sessionHeaderView(); got == "" {
		t.Fatal("header hidden with bottom view at 18-row boundary")
	}
}

func TestRuntimeResizePreservesScrolledConversationAndSingleComposer(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.runner = fakeConversation{}
	m.panes.bottom.setHasRunner(true)
	m.activeModel = "qwen3.8-27b"
	m.resize(100, 30)
	for i := 0; i < 100; i++ {
		m.appendLine("history")
	}
	m.refreshViewport()
	m.viewport.PageUp()
	m.conversationViewport.setFollowing(false)

	for _, size := range [][2]int{{60, 16}, {120, 32}, {72, 20}} {
		m.resize(size[0], size[1])
		if m.conversationViewport.following() {
			t.Fatalf("resize to %dx%d reset scrolled conversation to follow mode", size[0], size[1])
		}
		view := m.View().Content
		plain := ansi.Strip(view)
		if got := strings.Count(plain, "> "); got != 1 {
			t.Fatalf("composer count after resize to %dx%d = %d, want 1", size[0], size[1], got)
		}
		assertRenderedFrameFits(t, view, size[0], size[1])
	}
}

func assertRenderedFrameFits(t *testing.T, view string, width, height int) {
	t.Helper()
	if got := lipgloss.Height(view); got > height {
		t.Fatalf("view height = %d exceeds terminal height %d", got, height)
	}
	for lineNumber, line := range strings.Split(view, "\n") {
		if got := ansi.StringWidth(line); got > width {
			t.Fatalf("line %d width = %d exceeds terminal width %d: %q", lineNumber+1, got, width, ansi.Strip(line))
		}
	}
}
