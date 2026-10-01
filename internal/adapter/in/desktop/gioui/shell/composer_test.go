//go:build desktop || desktop_gio

package shell

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestComposerHelperPreservesImportantStatesWhenCompact(t *testing.T) {
	if got := composerHelper(true, false, false, true); got != "" {
		t.Fatalf("compact ready helper = %q, want hidden", got)
	}
	for _, test := range []struct {
		name      string
		busy      bool
		loading   bool
		connected bool
		want      string
	}{
		{name: "busy", busy: true, connected: true, want: "Use Stop to cancel the active turn"},
		{name: "loading", loading: true, connected: true, want: "Prompting resumes after session history finishes loading"},
		{name: "disconnected", connected: false, want: "Reconnect to the ACP runtime before sending"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := composerHelper(true, test.busy, test.loading, test.connected); got != test.want {
				t.Fatalf("compact helper = %q, want %q", got, test.want)
			}
		})
	}
	if got := composerHelper(false, false, false, true); got != "Enter sends · Shift+Enter adds a line" {
		t.Fatalf("wide ready helper = %q", got)
	}
}

func TestComposerLayoutFitsCompactAndWideConstraints(t *testing.T) {
	snapshot := desktopCaptureSnapshot()
	session := snapshot.State.Sessions[0]
	compactHeight := composerTestLayout(t, 330, 480, session, snapshot).Y
	wideHeight := composerTestLayout(t, 700, 480, session, snapshot).Y
	if compactHeight >= wideHeight {
		t.Fatalf("compact composer height = %d, wide composer height = %d; want compact to use less space", compactHeight, wideHeight)
	}
}

func composerTestLayout(t *testing.T, width, height int, session desktopstate.SessionState, snapshot controller.Snapshot) image.Point {
	t.Helper()
	view := New(NewTheme("light"), Bindings{})
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(width, 0), Max: image.Pt(width, height)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	dims := view.layoutComposer(gtx, session, snapshot)
	if dims.Size.X > width || dims.Size.Y > height {
		t.Fatalf("composer size = %v, exceeds %dx%d constraints", dims.Size, width, height)
	}
	router.Frame(gtx.Ops)
	return dims.Size
}

func TestConversationCacheKeyIsStableAcrossTrim(t *testing.T) {
	item := desktopstate.TimelineItem{Kind: desktopstate.TimelineAssistant, ID: "message-7", Text: "hello"}
	before := makeConversationCacheKey("session-1", 128, item)
	after := makeConversationCacheKey("session-1", 3, item)
	if before != after {
		t.Fatalf("cache key changed with index: before=%#v after=%#v", before, after)
	}

	anonymous := desktopstate.TimelineItem{Kind: desktopstate.TimelineAssistant, Text: "no id"}
	if makeConversationCacheKey("session-1", 1, anonymous) == makeConversationCacheKey("session-1", 2, anonymous) {
		t.Fatal("anonymous items with different indices shared a cache key")
	}
}

func TestCollapsibleToolAndDiffCards(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
	sessionID := "session-tools"

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(800, 0), Max: image.Pt(800, 600)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	toolItem := desktopstate.TimelineItem{
		Kind:   desktopstate.TimelineTool,
		ID:     "tool-1",
		Title:  "bash: go test ./...",
		Text:   "PASS\nok github.com/proton/test 0.05s",
		Status: "completed",
	}

	collapsedDims := view.layoutTimelineItem(gtx, sessionID, 0, toolItem)
	if collapsedDims.Size.Y <= 0 {
		t.Fatalf("collapsed tool item height = %d, want > 0", collapsedDims.Size.Y)
	}

	toolBtn := view.toolExpandButtons["tool-1"]
	if toolBtn == nil {
		t.Fatalf("toolExpandButtons[tool-1] was not initialized")
	}
	toolBtn.Click()
	expandedDims := view.layoutTimelineItem(gtx, sessionID, 0, toolItem)
	if expandedDims.Size.Y <= collapsedDims.Size.Y {
		t.Fatalf("expanded tool item height %d should be greater than collapsed %d", expandedDims.Size.Y, collapsedDims.Size.Y)
	}

	diffText := "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1,3 +1,4 @@\n package main\n+import \"fmt\"\n func main() {}\n"
	diffItem := desktopstate.TimelineItem{
		Kind:   desktopstate.TimelineTool,
		ID:     "diff-1",
		Title:  "edit: main.go",
		Text:   diffText,
		Status: "completed",
	}

	diffDims := view.layoutTimelineItem(gtx, sessionID, 1, diffItem)
	if diffDims.Size.Y <= 0 {
		t.Fatalf("diff item height = %d, want > 0", diffDims.Size.Y)
	}
}

func mkJumpGtx() layout.Context {
	ops := new(op.Ops)
	var router input.Router
	return layout.Context{
		Ops:         ops,
		Constraints: layout.Constraints{Min: image.Pt(800, 0), Max: image.Pt(800, 600)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
}

// Submitting must clear exactly the active session's draft and nothing else.
// Leaving the draft behind would replay a sent prompt the next time the user
// switches back, and clearing the wrong session would silently lose unsent text.
func TestSubmitComposerClearsOnlyTheActiveSessionDraft(t *testing.T) {
	var sent controller.ExpandedPrompt
	sh := New(NewTheme("light"), Bindings{SendPrompt: func(p controller.ExpandedPrompt) { sent = p }})
	// Drafts are keyed by the composite session reference so the same session id
	// under two agents cannot share (or clobber) a draft.
	keyA := controller.SessionRefStorageKey(desktopstate.SessionRef{AgentID: "protonman", SessionID: "session-a"})
	keyB := controller.SessionRefStorageKey(desktopstate.SessionRef{AgentID: "reviewer", SessionID: "session-b"})
	sh.rememberComposerDraft(keyA, "draft for a")
	sh.rememberComposerDraft(keyB, "draft for b")

	sh.activeSessionID = "session-a"
	sh.activeSessionAgentID = "protonman"
	sh.setComposerText("  ship it  ")
	sh.submitComposer("  ship it  ")

	if sent.TurnPrompt != "ship it" {
		t.Fatalf("submitted prompt = %q, want the trimmed text", sent.TurnPrompt)
	}
	if got := sh.takeComposerDraft(keyA); got != "" {
		t.Fatalf("submitting must clear the active draft, got %q", got)
	}
	if got := sh.takeComposerDraft(keyB); got != "draft for b" {
		t.Fatalf("submitting must not touch another session's draft, got %q", got)
	}
	if sh.conversationUI.Editor().Text() != "" {
		t.Fatalf("submitting must clear the editor, got %q", sh.conversationUI.Editor().Text())
	}
	if !sh.conversationUI.Timeline().ScrollToEnd {
		t.Fatal("submitting must return the transcript to the live tail")
	}
}

// A blank or whitespace-only submission must be a no-op: no prompt is sent and
// the existing draft survives, because the user may have meant to keep typing.
func TestSubmitComposerIgnoresBlankInput(t *testing.T) {
	var sends int
	sh := New(NewTheme("light"), Bindings{SendPrompt: func(controller.ExpandedPrompt) { sends++ }})
	key := controller.SessionRefStorageKey(desktopstate.SessionRef{AgentID: "protonman", SessionID: "session-a"})
	sh.rememberComposerDraft(key, "unsent")
	sh.activeSessionID = "session-a"

	for _, blank := range []string{"", "   ", "\n\t "} {
		sh.submitComposer(blank)
	}
	if sends != 0 {
		t.Fatalf("blank submissions sent %d prompts, want 0", sends)
	}
	if got := sh.takeComposerDraft(key); got != "unsent" {
		t.Fatalf("blank submission must not drop the draft, got %q", got)
	}
}

// The composer must grow with its content: a single-line composer reserves one
// row, and a multi-line draft reserves one row per line. Without this the editor
// stays visually one line tall and the extra lines are unreachable.
func TestComposerEditorAutoExpandsWithLines(t *testing.T) {
	snapshot := desktopCaptureSnapshot()
	session := snapshot.State.Sessions[0]
	sh := New(NewTheme("light"), Bindings{})

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(700, 0), Max: image.Pt(700, 600)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	emptyDims := sh.layoutComposer(gtx, session, snapshot)

	sh.conversationUI.Editor().SetText("Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6")
	multiLineDims := sh.layoutComposer(gtx, session, snapshot)

	if multiLineDims.Size.Y <= emptyDims.Size.Y {
		t.Fatalf("multi-line composer height = %d, want > empty height %d", multiLineDims.Size.Y, emptyDims.Size.Y)
	}
}
