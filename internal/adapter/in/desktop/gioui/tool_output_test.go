//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func toolOutputTestContext() (layout.Context, *op.Ops) {
	operations := new(op.Ops)
	return layout.Context{
		Ops:         operations,
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
	}, operations
}

func TestDiffToolItemsDefaultCollapsed(t *testing.T) {
	sh := newShell(newTheme("dark"))
	gtx, _ := toolOutputTestContext()

	item := desktopstate.TimelineItem{
		Kind:   desktopstate.TimelineTool,
		ID:     "tool-diff",
		Title:  "edit",
		Status: "completed",
		Text: "diff --git a/main.go b/main.go\n" +
			strings.Repeat("+added line\n", 8) +
			strings.Repeat("-removed line\n", 8),
	}

	sh.layoutToolItem(gtx, "session-1", 0, item, sh.theme.onSurface)

	if expanded, ok := sh.toolExpanded["tool-diff"]; ok && expanded {
		t.Fatal("diff tool items must default to collapsed")
	}
}

func TestDiffToolItemKeepsExplicitExpansion(t *testing.T) {
	sh := newShell(newTheme("dark"))
	gtx, _ := toolOutputTestContext()

	item := desktopstate.TimelineItem{
		Kind:   desktopstate.TimelineTool,
		ID:     "tool-diff",
		Title:  "edit",
		Status: "completed",
		Text:   "diff --git a/main.go b/main.go\n@@ -1,1 +1,1 @@\n-old\n+new",
	}

	sh.toolExpanded = map[string]bool{"tool-diff": true}
	sh.layoutToolItem(gtx, "session-1", 0, item, sh.theme.onSurface)

	if !sh.toolExpanded["tool-diff"] {
		t.Fatal("explicitly expanded diff items must stay expanded")
	}
}

func TestToolOutputWindowPagesClampAndAdvance(t *testing.T) {
	text := strings.Repeat("x", int(largeMessagePageBytes)*2+16)

	if _, pageCount, current := toolOutputWindow(text, -1); pageCount != 3 || current != 0 {
		t.Fatalf("negative page must clamp to 0: pages=%d current=%d", pageCount, current)
	}
	if _, pageCount, current := toolOutputWindow(text, 99); pageCount != 3 || current != 2 {
		t.Fatalf("out-of-range page must clamp to last: pages=%d current=%d", pageCount, current)
	}

	page := 1
	for range 5 {
		_, pageCount, current := toolOutputWindow(text, page)
		if current != page {
			t.Fatalf("page %d rendered as %d", page, current)
		}
		page++
		if page >= pageCount {
			page = pageCount - 1
		}
	}
}

func TestToolOutputWindowPreservesRuneBoundaries(t *testing.T) {
	text := strings.Repeat("é", int(largeMessagePageBytes)) // 2 bytes per rune

	paged, pageCount, _ := toolOutputWindow(text, 1)
	if pageCount != 2 {
		t.Fatalf("expected 2 pages, got %d", pageCount)
	}
	if !utf8.ValidString(paged) {
		t.Fatal("paged output must stay valid UTF-8")
	}
	if want := largeMessagePageBytes; len(paged) != want {
		t.Fatalf("page size = %d, want %d", len(paged), want)
	}
}

func TestLargeToolOutputsPageByDefault(t *testing.T) {
	sh := newShell(newTheme("dark"))
	gtx, _ := toolOutputTestContext()

	item := desktopstate.TimelineItem{
		Kind:   desktopstate.TimelineTool,
		ID:     "tool-big",
		Title:  "bash",
		Status: "completed",
		Text:   strings.Repeat("diagnostic line\n", int(maxToolOutputUnpagedBytes)/8),
	}
	sh.toolExpanded = map[string]bool{"tool-big": true}

	sh.layoutToolItem(gtx, "session-1", 0, item, sh.theme.onSurface)

	if expanded, ok := sh.conversationExpanded[makeConversationCacheKey("session-1", 0, item)]; ok && expanded {
		t.Fatal("large tool outputs must start in the preview state, not fully expanded")
	}
}

func TestSmallToolOutputsRenderUnpaged(t *testing.T) {
	sh := newShell(newTheme("dark"))
	gtx, _ := toolOutputTestContext()

	item := desktopstate.TimelineItem{
		Kind:   desktopstate.TimelineTool,
		ID:     "tool-small",
		Title:  "bash",
		Status: "completed",
		Text:   "short output",
	}
	sh.toolExpanded = map[string]bool{"tool-small": true}

	sh.layoutToolItem(gtx, "session-1", 0, item, sh.theme.onSurface)

	if expanded, ok := sh.conversationExpanded[makeConversationCacheKey("session-1", 0, item)]; ok && expanded {
		t.Fatal("small outputs must not reserve a paging slot in conversationExpanded")
	}
}

func TestSessionSwitchClearsToolOutputPagingState(t *testing.T) {
	sh := newShell(newTheme("dark"))
	key := conversationCacheKey{sessionID: "s1", itemID: "tool-1"}

	sh.toolOutputPages = map[conversationCacheKey]*toolOutputPageButtons{
		key: {},
	}
	sh.conversationExpanded = map[conversationCacheKey]bool{key: true}

	sh.syncConversation(desktopstate.State{ActiveSessionID: "s2"})

	if len(sh.toolOutputPages) != 0 {
		t.Fatalf("toolOutputPages must reset on session switch, got %d entries", len(sh.toolOutputPages))
	}
}

func TestToolOutputPageStateEviction(t *testing.T) {
	sh := newShell(newTheme("dark"))
	for index := range maxConversationExpansionKeys {
		sh.toolOutputPageState(conversationCacheKey{itemID: "tool-" + strconv.Itoa(index)})
	}
	if len(sh.toolOutputPages) != maxConversationExpansionKeys {
		t.Fatalf("expected %d entries, got %d", maxConversationExpansionKeys, len(sh.toolOutputPages))
	}

	sh.toolOutputPageState(conversationCacheKey{itemID: "overflow"})
	if len(sh.toolOutputPages) > maxConversationExpansionKeys {
		t.Fatalf("page-state map must stay bounded at %d, got %d", maxConversationExpansionKeys, len(sh.toolOutputPages))
	}
	if _, ok := sh.toolOutputPages[conversationCacheKey{itemID: "overflow"}]; !ok {
		t.Fatal("overflow entry must be present after eviction")
	}
}

// Eviction must be deterministic FIFO: the first key admitted is the first
// key dropped, regardless of map iteration order.
func TestExpansionEvictionIsDeterministicFIFO(t *testing.T) {
	sh := newShell(newTheme("dark"))
	first := conversationCacheKey{sessionID: "s", itemID: "first"}
	second := conversationCacheKey{sessionID: "s", itemID: "second"}

	sh.admitExpansionKey(first)
	for index := range maxConversationExpansionKeys - 1 {
		sh.admitExpansionKey(conversationCacheKey{sessionID: "s", itemID: "filler-" + strconv.Itoa(index)})
	}
	sh.admitExpansionKey(second) // full: first is oldest

	if _, ok := sh.expansionOrder.get(first); ok {
		t.Fatal("oldest expansion key must be evicted first")
	}
	if _, ok := sh.expansionOrder.get(second); !ok {
		t.Fatal("newest expansion key must survive")
	}
}

// Evicting an expansion key must clean every expansion-state map, not just
// the map that happened to trigger the eviction.
func TestExpansionEvictionClearsAllExpansionMaps(t *testing.T) {
	sh := newShell(newTheme("dark"))
	victim := conversationCacheKey{sessionID: "s", itemID: "victim"}

	// The victim registers through the tool-output path with page state.
	sh.toolOutputPageState(victim)
	sh.toolExpanded = map[string]bool{"victim-tool": true}

	// Seat it for the large-message path too, then churn past the bound.
	sh.largeMessageDisclosureButtons(victim)
	sh.conversationExpanded[victim] = true
	sh.conversationPage[victim] = 2

	for index := range maxConversationExpansionKeys {
		sh.largeMessageDisclosureButtons(conversationCacheKey{sessionID: "s", itemID: "msg-" + strconv.Itoa(index)})
	}

	if _, ok := sh.conversationExpandButtons[victim]; ok {
		t.Fatal("evicted key must leave conversationExpandButtons")
	}
	if _, ok := sh.toolOutputPages[victim]; ok {
		t.Fatal("evicted key must leave toolOutputPages")
	}
	if _, ok := sh.conversationExpanded[victim]; ok {
		t.Fatal("evicted key must leave conversationExpanded")
	}
	if _, ok := sh.conversationPage[victim]; ok {
		t.Fatal("evicted key must leave conversationPage")
	}
	if len(sh.conversationExpandButtons) > maxConversationExpansionKeys || len(sh.toolOutputPages) > maxConversationExpansionKeys {
		t.Fatal("expansion maps must stay within the shared bound")
	}
}
