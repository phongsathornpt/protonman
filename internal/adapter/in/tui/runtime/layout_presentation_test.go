package runtime

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/state/runtimeui"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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

func TestFlexibleViewportKeepsComposerPositionStableAcrossActivity(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)

	partHeight := func(part string) int {
		if part == "" {
			return 0
		}
		return strings.Count(part, "\n") + 1
	}
	idleViewportHeight := m.viewport.Height()
	idlePreComposerHeight := idleViewportHeight + partHeight(m.layout.frame.status)
	if m.layout.frame.status != "" {
		t.Fatalf("idle status = %q, want empty", m.layout.frame.status)
	}

	m.busy = true
	m.activity = "running tests"
	m.busyStarted = time.Now().Add(-time.Second)
	m.requestRelayout()
	m.reconcileLayout()

	busyViewportHeight := m.viewport.Height()
	busyPreComposerHeight := busyViewportHeight + partHeight(m.layout.frame.status)
	if m.layout.frame.status == "" {
		t.Fatal("busy frame should render a status row")
	}
	if busyPreComposerHeight != idlePreComposerHeight {
		t.Fatalf("pre-composer height changed from idle %d to busy %d", idlePreComposerHeight, busyPreComposerHeight)
	}
	if busyViewportHeight != idleViewportHeight-1 {
		t.Fatalf("busy viewport height = %d, want idle height %d minus one status row", busyViewportHeight, idleViewportHeight)
	}
}

func TestRuntimeStatusProjectionDistinguishesStreamingAndToolWork(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true
	now := time.Now()
	m.busyStarted = now.Add(-3 * time.Second)

	m.historyState.AppendAssistantDelta("streaming")
	streaming := m.runtimeStatusState(now)
	if streaming.Phase != runtimeui.PhaseStreaming || streaming.Activity != "streaming response" {
		t.Fatalf("streaming state = %#v", streaming)
	}

	m.historyState.StartTool("read")
	m.activity = "running read"
	m.turnProgress.ToolCalls = 2
	toolState := m.runtimeStatusState(now)
	if toolState.Phase != runtimeui.PhaseToolRunning || toolState.Activity != "running read" {
		t.Fatalf("tool state = %#v", toolState)
	}
	if got := toolState.MetaText(); !strings.Contains(got, "2 tools") || !strings.Contains(got, "3s") {
		t.Fatalf("tool meta = %q", got)
	}
}

func TestRuntimeStatusProjectionRetryOutranksToolWork(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.busy = true
	now := time.Now()
	m.historyState.StartTool("read")
	m.activity = "running read"
	m.turnProgress.Retry = sdk.RetryEvent{
		Attempt:    2,
		MaxRetries: 4,
		Reason:     "idle_event_timeout",
		RetryAt:    now.Add(2 * time.Second),
	}

	state := m.runtimeStatusState(now)
	if state.Phase != runtimeui.PhaseRetryWaiting || state.Activity != "retrying in 2s" {
		t.Fatalf("retry state = %#v", state)
	}
	if got := state.MetaText(); !strings.Contains(got, "retry 2/4") || !strings.Contains(got, "stream stalled") {
		t.Fatalf("retry meta = %q", got)
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
