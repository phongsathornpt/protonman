package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestSessionHeaderUsesSharedLayoutProfile(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.activeModel = "qwen3.8-27b"

	m.resize(80, 24)
	if got := len(strings.Split(ansi.Strip(m.sessionHeaderView()), "\n")); got != 4 {
		t.Fatalf("normal header lines = %d, want 4", got)
	}

	m.resize(32, 24)
	if got := len(strings.Split(ansi.Strip(m.sessionHeaderView()), "\n")); got != 2 {
		t.Fatalf("compact header lines = %d, want 2", got)
	}

	m.resize(20, 24)
	if got := len(strings.Split(ansi.Strip(m.sessionHeaderView()), "\n")); got != 1 {
		t.Fatalf("minimal header lines = %d, want 1", got)
	}
}

func TestFrameReservesStatusRowToKeepComposerStable(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	idleHeight := m.layout.frame.height
	idleViewportHeight := m.viewport.Height()
	if m.layout.frame.status == "" || strings.TrimSpace(m.layout.frame.status) != "" {
		t.Fatalf("idle frame should reserve a blank status row: %q", m.layout.frame.status)
	}

	m.busy = true
	m.activity = "running tests"
	m.busyStarted = time.Now().Add(-time.Second)
	m.requestRelayout()
	m.reconcileLayout()

	if got := m.layout.frame.height; got != idleHeight {
		t.Fatalf("frame height changed from idle %d to busy %d", idleHeight, got)
	}
	if got := m.viewport.Height(); got != idleViewportHeight {
		t.Fatalf("viewport height changed from idle %d to busy %d", idleViewportHeight, got)
	}
}

func TestStatusViewIncludesElapsedRuntime(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true
	m.activity = "running tests"
	m.busyStarted = time.Now().Add(-5 * time.Second)

	plain := ansi.Strip(m.statusView())
	if !strings.Contains(plain, "running tests") || !strings.Contains(plain, "5s") {
		t.Fatalf("status missing activity or elapsed runtime: %q", plain)
	}
}

func TestStatusViewFitsDisplayCellWidth(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(32, 24)
	m.busy = true
	m.activity = strings.Repeat("進捗", 20)

	got := m.statusView()
	wantMax := m.layoutProfile().ContentWidth(m.layout.width)
	if width := ansi.StringWidth(got); width > wantMax {
		t.Fatalf("status width = %d, want <= %d: %q", width, wantMax, ansi.Strip(got))
	}
}
