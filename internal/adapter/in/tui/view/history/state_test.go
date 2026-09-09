package history

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestScrollAnchorTracksCellAcrossEarlierExpansion(t *testing.T) {
	state := NewHistoryState(100)
	run := &AgentRunCell{AgentID: "worker-1", Profile: agent.ProfileStrength, Task: "fix", State: agent.StateRunning}
	target := &SystemCell{Text: "target"}
	state.Append(run)
	state.Append(target)
	anchor := state.CaptureScrollAnchor(2)
	run.Activity = "searching"
	state.TouchAgentRun("worker-1")
	got, ok := state.ResolveScrollAnchor(anchor)
	if !ok {
		t.Fatal("anchor was not resolved after earlier cell expansion")
	}
	if got != 3 {
		t.Fatalf("resolved line=%d, want 3", got)
	}
}

func TestScrollAnchorsMatchRenderedContentLines(t *testing.T) {
	state := NewHistoryState(100)
	state.Append(&SystemCell{Text: "one"})
	state.Append(&SystemCell{Text: "two"})
	state.active = &AssistantCell{Text: "three"}
	content := state.RenderContent()
	want := 0
	if content != "" {
		want = strings.Count(content, "\n") + 1
	}
	if got := len(state.ScrollAnchors()); got != want {
		t.Fatalf("ScrollAnchors lines=%d RenderContent lines=%d content=%q", got, want, content)
	}
}

type countingCell struct {
	text    string
	renders int
}

func (*countingCell) Kind() HistoryCellKind { return HistoryCellSystem }
func (c *countingCell) RenderWidth(int) []string {
	c.renders++
	return []string{c.text}
}
func (c *countingCell) RawLines() []string { return []string{c.text} }
func (*countingCell) LineCount() int       { return 1 }

func TestScrollMetadataReusesCommittedRenderCache(t *testing.T) {
	state := NewHistoryState(100)
	cell := &countingCell{text: "cached"}
	state.Append(cell)
	_ = state.RenderContent()
	renders := cell.renders
	anchor := state.CaptureScrollAnchor(0)
	_ = state.ScrollAnchors()
	if _, ok := state.ResolveScrollAnchor(anchor); !ok {
		t.Fatal("cached anchor did not resolve")
	}
	if cell.renders != renders {
		t.Fatalf("scroll metadata rerendered committed cell: before=%d after=%d", renders, cell.renders)
	}
}
func TestDiscardToolCallClearsRemovedBackingSlot(t *testing.T) {
	state := NewHistoryState(100)
	state.committed = make([]HistoryCell, 0, 4)
	state.committed = append(state.committed,
		&SystemCell{Text: "before"},
		&ToolCell{CallID: "call-1", Name: "read", Running: true},
		&SystemCell{Text: "after"},
	)
	state.committedLines = 3

	if !state.DiscardToolCall("call-1", "read") {
		t.Fatal("expected running tool to be discarded")
	}
	if len(state.committed) != 2 {
		t.Fatalf("committed len=%d, want 2", len(state.committed))
	}
	backing := state.committed[:3]
	if backing[2] != nil {
		t.Fatalf("removed backing slot still retains %#v", backing[2])
	}
}

func TestCommittedAssistantReleasesPerCellRenderCache(t *testing.T) {
	state := NewHistoryState(100)
	assistant := &AssistantCell{Text: "first line\nsecond line\nthird line"}
	state.Append(assistant)

	first := state.RenderContent()
	if first == "" {
		t.Fatal("expected rendered assistant content")
	}
	if len(assistant.renderCache.lines) != 0 || len(assistant.renderCache.decorated) != 0 || assistant.renderCache.processed != 0 {
		t.Fatalf("committed assistant retained render cache: %+v", assistant.renderCache)
	}

	state.SetWidth(40)
	second := state.RenderContent()
	if second == "" {
		t.Fatal("expected rerendered assistant content after width change")
	}
	if len(assistant.renderCache.lines) != 0 || len(assistant.renderCache.decorated) != 0 || assistant.renderCache.processed != 0 {
		t.Fatalf("width rerender retained per-cell cache: %+v", assistant.renderCache)
	}
}

func TestReleaseRawTextCacheDropsMaterializedTranscript(t *testing.T) {
	state := NewHistoryState(100)
	state.Append(&AssistantCell{Text: strings.Repeat("raw transcript ", 256)})
	if got := state.Raw(); got == "" {
		t.Fatal("expected raw transcript")
	}
	if !state.rawTextValid || state.cachedRawText == "" {
		t.Fatal("expected raw transcript cache to be populated")
	}
	state.ReleaseRawTextCache()
	if state.rawTextValid || state.cachedRawText != "" {
		t.Fatalf("raw transcript cache retained state: valid=%v bytes=%d", state.rawTextValid, len(state.cachedRawText))
	}
}

func TestReleaseAlternateRenderCacheDropsRenderedTranscript(t *testing.T) {
	state := NewHistoryState(100)
	state.Append(&AssistantCell{Text: "cached alternate transcript"})
	_ = state.RenderLinesAt(40)
	if len(state.altRender) == 0 || !state.altRenderValid {
		t.Fatal("expected alternate render cache to be populated")
	}
	state.ReleaseAlternateRenderCache()
	if state.altRender != nil || state.altRenderValid || state.altRenderWidth != 0 {
		t.Fatalf("alternate render cache retained state: len=%d valid=%v width=%d", len(state.altRender), state.altRenderValid, state.altRenderWidth)
	}
}

func TestResetReleasesCommittedBackingStore(t *testing.T) {
	state := NewHistoryState(100)
	state.committed = make([]HistoryCell, 0, 64)
	state.Append(&AssistantCell{Text: strings.Repeat("x", 1024)})
	state.Append(&ToolCell{Name: "read", Body: strings.Repeat("y", 1024)})
	if cap(state.committed) == 0 {
		t.Fatal("expected committed backing storage before reset")
	}
	state.Reset()
	if state.committed != nil || cap(state.committed) != 0 {
		t.Fatalf("reset retained committed backing storage: len=%d cap=%d", len(state.committed), cap(state.committed))
	}
}

func TestRoutineToolAggregationKeepsRawFidelity(t *testing.T) {
	state := NewHistoryState(100)
	state.Append(&ToolCell{Name: "read", Target: "a.go", ToolKind: tool.KindRead, Body: "a"})
	state.Append(&ToolCell{Name: "read", Target: "b.go", ToolKind: tool.KindRead, Body: "b"})
	state.Append(&ToolCell{Name: "read", Target: "c.go", ToolKind: tool.KindRead, Body: "c"})
	rich := state.RenderContent()
	if !strings.Contains(rich, "Read 3 files") || strings.Contains(rich, "a.go") {
		t.Fatalf("rich aggregation=%q", rich)
	}
	raw := state.Raw()
	for _, target := range []string{"a.go", "b.go", "c.go"} {
		if !strings.Contains(raw, target) {
			t.Fatalf("raw transcript lost %s: %q", target, raw)
		}
	}
}

func TestRoutineToolAggregationStopsAtFailure(t *testing.T) {
	state := NewHistoryState(100)
	state.Append(&ToolCell{Name: "read", Target: "a.go", ToolKind: tool.KindRead})
	state.Append(&ToolCell{Name: "read", Target: "bad.go", ToolKind: tool.KindRead, FailureCode: tool.ErrorCodeNotFound, Body: "missing"})
	state.Append(&ToolCell{Name: "read", Target: "c.go", ToolKind: tool.KindRead})
	rich := state.RenderContent()
	if strings.Contains(rich, "Read 3 files") || !strings.Contains(rich, "bad.go") {
		t.Fatalf("failure was incorrectly aggregated: %q", rich)
	}
}

func TestRoutineToolAggregationPreservesScrollAnchor(t *testing.T) {
	state := NewHistoryState(100)
	first := &ToolCell{Name: "read", Target: "a.go", ToolKind: tool.KindRead}
	second := &ToolCell{Name: "read", Target: "b.go", ToolKind: tool.KindRead}
	after := &SystemCell{Text: "after"}
	state.Append(first)
	state.Append(second)
	state.Append(after)
	lines := state.RenderLines()
	if len(lines) < 3 {
		t.Fatalf("unexpected aggregated render: %#v", lines)
	}
	anchor := state.CaptureScrollAnchor(len(lines) - 1)
	if resolved, ok := state.ResolveScrollAnchor(anchor); !ok || resolved != len(lines)-1 {
		t.Fatalf("anchor resolve=(%d,%v), want %d", resolved, ok, len(lines)-1)
	}
}
