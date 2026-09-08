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
