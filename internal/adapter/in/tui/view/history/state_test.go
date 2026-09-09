package history

import (
	"strings"
	"testing"

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
func (c *countingCell) Render() []string    { return c.RenderWidth(defaultHistoryWidth) }
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
func TestSpinnerFrameUpdatesCommittedCacheInPlace(t *testing.T) {
	state := NewHistoryState(100)
	state.StartTool("read_file")
	state.StartTool("grep")
	_ = state.RenderContent()
	if !state.cacheValid {
		t.Fatal("expected committed render cache to be valid")
	}
	beforeRevision, _ := state.Revisions()
	if !state.SetSpinnerFrame("⠙") {
		t.Fatal("expected running committed tool to consume spinner frame")
	}
	if !state.cacheValid {
		t.Fatal("spinner frame invalidated full committed render cache")
	}
	afterRevision, _ := state.Revisions()
	if afterRevision != beforeRevision {
		t.Fatalf("spinner changed committed semantic revision: before=%d after=%d", beforeRevision, afterRevision)
	}
	if content := state.RenderContent(); !strings.Contains(content, "⠙") {
		t.Fatalf("cached transcript did not reflect spinner update: %q", content)
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
