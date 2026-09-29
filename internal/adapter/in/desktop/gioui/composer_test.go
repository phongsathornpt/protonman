//go:build desktop || desktop_gio

package gioui

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestComposerDraftsStayWithTheirSession(t *testing.T) {
	view := newShell(newTheme("light"))
	view.syncConversation(desktopstate.State{ActiveSessionID: "first"})
	view.composer.SetText("first session draft")

	view.syncConversation(desktopstate.State{ActiveSessionID: "second"})
	if got := view.composer.Text(); got != "" {
		t.Fatalf("new session composer = %q, want empty", got)
	}
	view.composer.SetText("second session draft")

	view.syncConversation(desktopstate.State{ActiveSessionID: "first"})
	if got := view.composer.Text(); got != "first session draft" {
		t.Fatalf("restored first session composer = %q", got)
	}
	view.syncConversation(desktopstate.State{ActiveSessionID: "second"})
	if got := view.composer.Text(); got != "second session draft" {
		t.Fatalf("restored second session composer = %q", got)
	}
}

func TestSubmitComposerTrimsAndClearsDraft(t *testing.T) {
	view := newShell(newTheme("light"))
	view.activeSessionID = "session"
	var submitted []string
	view.onSendPrompt = func(p ExpandedPrompt) { submitted = append(submitted, p.DisplayText) }
	view.composer.SetText("saved draft")
	view.rememberComposerDraft("session", "older draft")

	view.submitComposer("  send this\n")
	if len(submitted) != 1 || submitted[0] != "send this" {
		t.Fatalf("submitted prompts = %#v, want [send this]", submitted)
	}
	if got := view.composer.Text(); got != "" {
		t.Fatalf("composer after submit = %q, want empty", got)
	}
	if got := view.takeComposerDraft("session"); got != "" {
		t.Fatalf("restored submitted draft = %q, want empty", got)
	}
}

func TestComposerDraftCacheIsBounded(t *testing.T) {
	view := newShell(newTheme("light"))
	for index := 0; index <= maxComposerDrafts; index++ {
		id := fmt.Sprintf("session-%02d", index)
		view.rememberComposerDraft(id, "draft")
	}
	if len(view.composerDrafts) != maxComposerDrafts || len(view.composerDraftOrder) != maxComposerDrafts {
		t.Fatalf("draft cache sizes = %d map entries, %d order entries; want %d", len(view.composerDrafts), len(view.composerDraftOrder), maxComposerDrafts)
	}
	if got := view.takeComposerDraft("session-00"); got != "" {
		t.Fatalf("oldest draft = %q, want evicted", got)
	}
	if got := view.takeComposerDraft(fmt.Sprintf("session-%02d", maxComposerDrafts)); got != "draft" {
		t.Fatalf("newest draft = %q, want retained", got)
	}
}

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

func composerTestLayout(t *testing.T, width, height int, session desktopstate.SessionState, snapshot controllerSnapshot) image.Point {
	t.Helper()
	view := newShell(newTheme("light"))
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

func TestComposerEditorAutoExpandsWithLines(t *testing.T) {
	snapshot := desktopCaptureSnapshot()
	session := snapshot.State.Sessions[0]

	view := newShell(newTheme("light"))
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(700, 0), Max: image.Pt(700, 600)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	emptyDims := view.layoutComposer(gtx, session, snapshot)

	view.composer.SetText("Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6")
	multiLineDims := view.layoutComposer(gtx, session, snapshot)

	if multiLineDims.Size.Y <= emptyDims.Size.Y {
		t.Fatalf("multi-line composer height = %d, want > empty height %d", multiLineDims.Size.Y, emptyDims.Size.Y)
	}
}

func TestEmptyStateStarterPromptCards(t *testing.T) {
	view := newShell(newTheme("dark"))
	session := desktopstate.SessionState{
		ID:        "session-empty",
		Workspace: "/path/to/project",
		AgentID:   "Protonman",
		Runtime:   desktopstate.RuntimeSettingsState{Model: "mimo-v2.6-pro"},
	}

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(800, 0), Max: image.Pt(800, 600)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	dims := view.layoutEmptyState(gtx, session)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("layoutEmptyState returned invalid dims: %v", dims)
	}

	// Verify clicking first starter card sets composer text and focuses it
	view.starterPromptButtons[0].Click()
	_ = view.layoutEmptyState(gtx, session)
	if got := view.composer.Text(); got != starterPrompts[0].prompt {
		t.Fatalf("composer text after clicking card 0 = %q, want %q", got, starterPrompts[0].prompt)
	}

	// Verify clicking another starter card replaces composer text
	view.starterPromptButtons[2].Click()
	_ = view.layoutEmptyState(gtx, session)
	if got := view.composer.Text(); got != starterPrompts[2].prompt {
		t.Fatalf("composer text after clicking card 2 = %q, want %q", got, starterPrompts[2].prompt)
	}
}

func TestMessageActionButtons(t *testing.T) {
	view := newShell(newTheme("dark"))
	sessionID := "session-actions"

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(800, 0), Max: image.Pt(800, 600)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	userItem := desktopstate.TimelineItem{
		Kind: desktopstate.TimelineUser,
		Text: "Can you fix the login bug?",
	}
	userDims := view.layoutTimelineItem(gtx, sessionID, 0, userItem)
	if userDims.Size.Y <= 0 {
		t.Fatalf("user timeline item height = %d, want > 0", userDims.Size.Y)
	}

	userMsgKey := conversationButtonKey(sessionID, "user", 0, userItem)
	retryBtn := view.userRetryButtons[userMsgKey]
	if retryBtn == nil {
		t.Fatalf("userRetryButtons[%q] was not initialized", userMsgKey)
	}
	retryBtn.Click()
	_ = view.layoutTimelineItem(gtx, sessionID, 0, userItem)
	if got := view.composer.Text(); got != userItem.Text {
		t.Fatalf("composer text after clicking retry = %q, want %q", got, userItem.Text)
	}

	asstItem := desktopstate.TimelineItem{
		Kind: desktopstate.TimelineAssistant,
		Text: "Here is the fix for the login bug.",
	}
	asstDims := view.layoutTimelineItem(gtx, sessionID, 1, asstItem)
	if asstDims.Size.Y <= 0 {
		t.Fatalf("asst timeline item height = %d, want > 0", asstDims.Size.Y)
	}

	asstMsgKey := conversationButtonKey(sessionID, "asst", 1, asstItem)
	copyBtn := view.messageCopyButtons[asstMsgKey]
	if copyBtn == nil {
		t.Fatalf("messageCopyButtons[%q] was not initialized", asstMsgKey)
	}
	copyBtn.Click()
	_ = view.layoutTimelineItem(gtx, sessionID, 1, asstItem)
	if _, ok := view.messageCopiedAt[asstMsgKey]; !ok {
		t.Fatalf("messageCopiedAt[%q] was not set after clicking copy", asstMsgKey)
	}
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
	view := newShell(newTheme("dark"))
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

func TestComposerTopHeaderAndPopoverLayout(t *testing.T) {
	snapshot := desktopCaptureSnapshot()
	session := snapshot.State.Sessions[0]
	session.Context.Goal = "Optimize desktop UI components"

	view := newShell(newTheme("light"))
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(700, 0), Max: image.Pt(700, 600)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	dims := view.layoutComposer(gtx, session, snapshot)
	if dims.Size.Y <= 0 {
		t.Fatalf("composer height = %d, want > 0", dims.Size.Y)
	}

	// Popover open: popover is displayed above composer without breaking constraints
	view.modelPopoverVisible = true
	popoverDims := view.layoutComposer(gtx, session, snapshot)
	if popoverDims.Size.Y <= dims.Size.Y {
		t.Fatalf("popover composer height = %d should be greater than base composer height %d", popoverDims.Size.Y, dims.Size.Y)
	}
}

func TestJumpToBottomSuppressedWhenPopoverOpen(t *testing.T) {
	view := newShell(newTheme("dark"))
	timeline := make([]desktopstate.TimelineItem, 0, 40)
	for i := 0; i < 40; i++ {
		kind := desktopstate.TimelineUser
		if i%2 == 1 {
			kind = desktopstate.TimelineAssistant
		}
		timeline = append(timeline, desktopstate.TimelineItem{
			Kind: kind,
			Text: strings.Repeat("Scrollable message content. ", 8),
		})
	}
	session := desktopstate.SessionState{
		ID:       "session-scroll",
		Timeline: timeline,
	}
	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Constraints{Min: image.Pt(800, 0), Max: image.Pt(800, 600)},
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	// Scrolled away from the tail: user intent is BeforeEnd, so layout does
	// not force-follow while reading older content.
	view.conversationList.ScrollToEnd = true
	view.conversationList.Position = layout.Position{First: 0, Offset: 0, BeforeEnd: true}
	_ = view.layoutConversation(gtx, session, historyStateLoaded)
	if view.conversationList.ScrollToEnd {
		t.Fatal("ScrollToEnd must stay false while scrolled away from the tail")
	}

	// Popover open suppresses auto-scroll follow, but the jump button stays
	// available; clicking it closes the overlay and follows the tail.
	view.modelPopoverVisible = true
	view.mentionActive = false
	view.conversationList.Position.BeforeEnd = false
	_ = view.layoutConversation(gtx, session, historyStateLoaded)
	if view.conversationList.ScrollToEnd {
		t.Fatal("ScrollToEnd must stay false while a composer overlay is open")
	}

	view.jumpToBottomButton.Click()
	_ = view.layoutConversation(gtx, session, historyStateLoaded)
	_ = view.layoutJumpToBottomButton(mkJumpGtx())
	if view.modelPopoverVisible {
		t.Fatal("jump-to-bottom must close composer overlays")
	}
	if !view.conversationList.ScrollToEnd {
		t.Fatal("jump-to-bottom must follow the tail after closing overlays")
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

func TestComposerOverlayExclusivity(t *testing.T) {
	view := newShell(newTheme("dark"))
	session := desktopstate.SessionState{ID: "session-overlay"}

	if view.modelPopoverVisible || view.reasoningPopoverVisible || view.mentionActive {
		t.Fatal("expected overlays initially closed")
	}

	view.mentionActive = true
	view.mentionItems = []MentionItem{{Kind: MentionItemKindAgent, Name: "strength"}}
	view.openModelPopover()
	if !view.modelPopoverVisible || view.reasoningPopoverVisible || view.mentionActive {
		t.Fatalf("model popover must close mentions, got model=%v reasoning=%v mention=%v",
			view.modelPopoverVisible, view.reasoningPopoverVisible, view.mentionActive)
	}

	view.mentionActive = true
	view.mentionItems = []MentionItem{{Kind: MentionItemKindAgent, Name: "strength"}}
	view.openReasoningPopover()
	if view.modelPopoverVisible || !view.reasoningPopoverVisible || view.mentionActive {
		t.Fatalf("reasoning popover must close mentions, got model=%v reasoning=%v mention=%v",
			view.modelPopoverVisible, view.reasoningPopoverVisible, view.mentionActive)
	}

	view.modelPopoverVisible = true
	view.syncConversation(desktopstate.State{ActiveSessionID: "session-overlay"})
	if view.modelPopoverVisible || view.reasoningPopoverVisible {
		t.Fatalf("session switch must close popovers, got model=%v reasoning=%v",
			view.modelPopoverVisible, view.reasoningPopoverVisible)
	}

	snapshot := desktopCaptureSnapshot()
	mkGtx := func() (layout.Context, *op.Ops, *input.Router) {
		var ops op.Ops
		var router input.Router
		return layout.Context{
			Ops:         &ops,
			Constraints: layout.Constraints{Min: image.Pt(700, 0), Max: image.Pt(700, 600)},
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}, &ops, &router
	}
	gtx, _, router := mkGtx()
	_ = session
	view.activeWorkspace = ""
	view.modelPopoverVisible = true
	view.composer.SetText("hello @str")
	view.composer.SetCaret(len([]rune("hello @str")), len([]rune("hello @str")))
	_ = view.layoutComposer(gtx, snapshot.State.Sessions[0], snapshot)
	router.Frame(gtx.Ops)
	if !view.mentionActive {
		t.Fatal("mention popup must activate for '@str' at caret")
	}
	if len(view.mentionItems) == 0 {
		t.Fatal("mention popup must offer matches for '@str'")
	}
	if view.modelPopoverVisible || view.reasoningPopoverVisible {
		t.Fatalf("mention activation must close popovers, got model=%v reasoning=%v",
			view.modelPopoverVisible, view.reasoningPopoverVisible)
	}
}
