//go:build desktop || desktop_gio

package shell

import (
	"image"
	"image/color"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/x/richtext"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestMainEmptyStateCopy(t *testing.T) {
	tests := []struct {
		name      string
		state     desktopstate.State
		wantTitle string
		wantReady bool
	}{
		{name: "no projects", wantTitle: "Start with a project"},
		{name: "project not selected", state: desktopstate.State{Projects: []desktopstate.ProjectState{{ID: "p1"}}}, wantTitle: "Select a project"},
		{name: "project selected", state: desktopstate.State{ActiveProjectID: "p1", Projects: []desktopstate.ProjectState{{ID: "p1"}}}, wantTitle: "Your workspace is ready", wantReady: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			title, body, ready := mainEmptyStateCopy(test.state)
			if title != test.wantTitle {
				t.Fatalf("title = %q, want %q", title, test.wantTitle)
			}
			if ready != test.wantReady {
				t.Fatalf("ready = %t, want %t", ready, test.wantReady)
			}
			if body == "" {
				t.Fatal("empty-state body must explain the next step")
			}
			if test.name == "no projects" && !strings.Contains(body, "adding a new project or folder isn’t available") {
				t.Fatalf("no-project guidance omits setup limitation: %q", body)
			}
		})
	}
}

func TestMainEmptyStateWorkspaceReadyCentering(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
	snapshot := controller.Snapshot{
		State: desktopstate.State{
			ActiveProjectID: "proj-1",
			Projects: []desktopstate.ProjectState{
				{ID: "proj-1", Name: "Protonman"},
			},
			Sessions: nil,
		},
		Connection: controller.ConnectionConnected,
	}

	const width, height = 1200, 800
	win, err := headless.NewWindow(width, height)
	if err != nil {
		t.Fatalf("headless.NewWindow failed: %v", err)
	}
	defer win.Release()

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(width, height)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	dims := view.layoutMain(gtx, snapshot)
	if dims.Size.X != width || dims.Size.Y != height {
		t.Fatalf("layoutMain returned dims = %v, want %dx%d", dims.Size, width, height)
	}

	if err := win.Frame(gtx.Ops); err != nil {
		t.Fatalf("win.Frame failed: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := win.Screenshot(img); err != nil {
		t.Fatalf("win.Screenshot failed: %v", err)
	}

	// In dark theme, surface is #1e1e20, surfaceContainer is #252527.
	// When properly centered, the left margin and right margin outside the card must be surface color.
	leftColor := img.RGBAAt(50, height/2)
	rightColor := img.RGBAAt(width-50, height/2)
	centerColor := img.RGBAAt(width/2, height/2)

	surfaceNRGBA := view.theme.Colors.Surface
	surfaceColor := color.RGBA(surfaceNRGBA)
	if leftColor != surfaceColor {
		t.Fatalf("left margin pixel at (50, %d) = %+v, want surface %+v (card is not centered)", height/2, leftColor, surfaceColor)
	}
	if rightColor != surfaceColor {
		t.Fatalf("right margin pixel at (%d, %d) = %+v, want surface %+v (card is not centered)", width-50, height/2, rightColor, surfaceColor)
	}
	if centerColor == surfaceColor {
		t.Fatalf("center pixel at (%d, %d) matches surface %+v, expected card surfaceContainer", width/2, height/2, centerColor)
	}
}

func TestMainEmptyStateWorkspaceReadyFullShellCentering(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
	snapshot := controller.Snapshot{
		State: desktopstate.State{
			ActiveProjectID: "proj-1",
			Projects: []desktopstate.ProjectState{
				{ID: "proj-1", Name: "Protonman"},
			},
			Sessions: nil,
		},
		Connection: controller.ConnectionConnected,
	}

	const width, height = 1200, 800
	win, err := headless.NewWindow(width, height)
	if err != nil {
		t.Fatalf("headless.NewWindow failed: %v", err)
	}
	defer win.Release()

	var ops op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &ops,
		Constraints: layout.Exact(image.Pt(width, height)),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}

	dims := view.Layout(gtx, snapshot)
	if dims.Size.X != width || dims.Size.Y != height {
		t.Fatalf("view.layout returned dims = %v, want %dx%d", dims.Size, width, height)
	}

	if err := win.Frame(gtx.Ops); err != nil {
		t.Fatalf("win.Frame failed: %v", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	if err := win.Screenshot(img); err != nil {
		t.Fatalf("win.Screenshot failed: %v", err)
	}

	// Main pane starts after sidebar (280dp) + divider (1dp) = 281px.
	// Main pane width = 1200 - 281 = 919px. Center of main pane = 281 + 919/2 = 740px.
	// Check left of card in main pane (X = 320, Y = 400), center (X = 740, Y = 400), right of card (X = 1150, Y = 400).
	leftColor := img.RGBAAt(320, 400)
	centerColor := img.RGBAAt(740, 400)
	rightColor := img.RGBAAt(1150, 400)

	surfaceNRGBA := view.theme.Colors.Surface
	surfaceColor := color.RGBA(surfaceNRGBA)
	if leftColor != surfaceColor {
		t.Fatalf("main pane left margin pixel at (320, 400) = %+v, want surface %+v", leftColor, surfaceColor)
	}
	if rightColor != surfaceColor {
		t.Fatalf("main pane right margin pixel at (1150, 400) = %+v, want surface %+v", rightColor, surfaceColor)
	}
	if centerColor == surfaceColor {
		t.Fatalf("main pane center pixel at (740, 400) matches surface %+v, expected card surfaceContainer", centerColor)
	}
}

func TestDesktopStreamingRenderRetentionSoak(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	snapshot := benchmarkShellSnapshot()
	snapshot.State.Sessions[0].Timeline = []desktopstate.TimelineItem{{
		ID: "assistant-stream", Kind: desktopstate.TimelineAssistant, Streaming: true,
	}}
	variants := make([]string, 32)
	for index := range variants {
		variants[index] = strings.Repeat("streaming assistant **markdown** ", 128) + strconv.Itoa(index)
	}
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(1180, 760)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.Layout(gtx, snapshot)
	router.Frame(gtx.Ops)
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	previousRetained := baseline.HeapAlloc
	for cycle := range 3 {
		for frame := range 5000 {
			snapshot.State.Sessions[0].Timeline[0].Text = variants[frame%len(variants)]
			snapshot.Revision++
			operations.Reset()
			view.Layout(gtx, snapshot)
			router.Frame(gtx.Ops)
		}
		runtime.GC()
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(view)
		runtime.KeepAlive(snapshot)
		runtime.KeepAlive(router)
		delta := int64(after.HeapAlloc) - int64(previousRetained)
		t.Logf("render soak cycle=%d retained_heap=%d delta=%d bytes", cycle+1, after.HeapAlloc, delta)
		if cycle > 0 {
			if delta < 0 {
				delta = -delta
			}
			if delta > 8<<20 {
				t.Fatalf("retained heap changed by %d bytes across render soak cycles", delta)
			}
		}
		previousRetained = after.HeapAlloc
	}
}

func TestDesktopCompletedOversizedMessageRenderRetentionSoak(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	snapshot := benchmarkShellSnapshot()
	textA := strings.Repeat("completed response **markdown** a ", 1<<15)
	textB := strings.Repeat("completed response **markdown** b ", 1<<15)
	snapshot.State.Sessions[0].Timeline = []desktopstate.TimelineItem{{
		ID: "assistant-completed", Kind: desktopstate.TimelineAssistant, Text: textA,
	}}
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(1180, 760)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.Layout(gtx, snapshot)
	router.Frame(gtx.Ops)
	key := makeConversationCacheKey(snapshot.State.ActiveSessionID, 0, snapshot.State.Sessions[0].Timeline[0])
	runtime.GC()
	var baseline runtime.MemStats
	runtime.ReadMemStats(&baseline)

	previousRetained := baseline.HeapAlloc
	for cycle := range 3 {
		for frame := range 1000 {
			if frame%2 == 0 {
				snapshot.State.Sessions[0].Timeline[0].Text = textB
				view.conversationExpanded[key] = true
				view.conversationPage[key] = 1
			} else {
				snapshot.State.Sessions[0].Timeline[0].Text = textA
				view.conversationExpanded[key] = false
				view.conversationPage[key] = 0
			}
			snapshot.Revision++
			operations.Reset()
			view.Layout(gtx, snapshot)
			router.Frame(gtx.Ops)
		}
		runtime.GC()
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		runtime.KeepAlive(view)
		runtime.KeepAlive(snapshot)
		runtime.KeepAlive(router)
		runtime.KeepAlive(textA)
		runtime.KeepAlive(textB)
		delta := int64(after.HeapAlloc) - int64(previousRetained)
		t.Logf("completed-message render soak cycle=%d retained_heap=%d delta=%d bytes", cycle+1, after.HeapAlloc, delta)
		if cycle > 0 {
			if delta < 0 {
				delta = -delta
			}
			if delta > 8<<20 {
				t.Fatalf("retained heap changed by %d bytes across completed-message render cycles", delta)
			}
		}
		previousRetained = after.HeapAlloc
	}
}

func TestLargeMessagePreviewIsBoundedAndUTF8Safe(t *testing.T) {
	source := strings.Repeat("界", maxMessagePreviewBytes)
	preview := largeMessagePreview(source)
	if !utf8.ValidString(preview) {
		t.Fatal("large message preview contains invalid UTF-8")
	}
	if len(preview) > maxMessagePreviewBytes {
		t.Fatalf("preview bytes = %d, limit = %d", len(preview), maxMessagePreviewBytes)
	}
	// The preview is a zero-copy substring of the source; truncation is
	// signaled by the "Show full message" affordance rather than an appended
	// marker, which kept a per-frame string allocation off the render path.
	if !strings.HasPrefix(source, preview) {
		t.Fatal("large message preview must be a prefix of its source")
	}
	if got := largeMessagePreview("short message"); got != "short message" {
		t.Fatalf("short preview = %q", got)
	}

	windowSource := strings.Repeat("界", messagePageBytes*2) + " tail"
	pageCount := (len(windowSource) + messagePageBytes - 1) / messagePageBytes
	var reconstructed strings.Builder
	previousEnd := 0
	for page := range pageCount {
		window, start, end := largeMessageWindow(windowSource, page)
		if start != previousEnd || end < start || end-start > messagePageBytes+utf8.UTFMax {
			t.Fatalf("page %d bounds = (%d,%d), previous end = %d", page, start, end, previousEnd)
		}
		if !utf8.ValidString(window) {
			t.Fatalf("page %d contains invalid UTF-8", page)
		}
		reconstructed.WriteString(window)
		previousEnd = end
	}
	if reconstructed.String() != windowSource {
		t.Fatal("large-message pages did not preserve the complete source")
	}
}

func TestSyncConversationReleasesLargeMessageDisclosureState(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	key := conversationCacheKey{sessionID: "old-session", itemID: "large-item"}
	view.conversationExpanded[key] = true
	view.conversationPage[key] = 2
	view.conversationExpandButtons[key] = &conversationDisclosureButtons{}

	view.syncConversation(desktopstate.State{ActiveSessionID: "new-session"})
	if len(view.conversationExpanded) != 0 || len(view.conversationPage) != 0 || len(view.conversationExpandButtons) != 0 {
		t.Fatalf("session switch retained large-message disclosure state: expanded=%d pages=%d buttons=%d", len(view.conversationExpanded), len(view.conversationPage), len(view.conversationExpandButtons))
	}
}

func TestSyncConversationRetainsBoundedRenderCachesAcrossSessionSwitches(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	markdownKey := conversationCacheKey{sessionID: "old-session", itemID: "assistant", kind: desktopstate.TimelineAssistant}
	markdown := conversationMarkdownCache{source: "cached markdown", bytes: 64}
	codeKey := conversationCacheKey{sessionID: "old-session", itemID: "code", kind: desktopstate.TimelineAssistant}
	code := conversationCodeCache{source: "cached code", lang: "go", bytes: 48}
	view.conversationCache[markdownKey] = markdown
	view.conversationCacheBytes = markdown.bytes
	view.conversationCacheOrder = []conversationCacheKey{markdownKey}
	view.conversationCodeCache[codeKey] = code
	view.conversationCodeCacheBytes = code.bytes
	view.conversationCodeCacheOrder = []conversationCacheKey{codeKey}

	view.syncConversation(desktopstate.State{ActiveSessionID: "old-session"})
	view.syncConversation(desktopstate.State{ActiveSessionID: "new-session"})

	if got, ok := view.conversationCache[markdownKey]; !ok || got.source != markdown.source {
		t.Fatal("session switch discarded the bounded markdown render cache")
	}
	if got, ok := view.conversationCodeCache[codeKey]; !ok || got.source != code.source {
		t.Fatal("session switch discarded the bounded code render cache")
	}
	if view.conversationCacheBytes != markdown.bytes || view.conversationCodeCacheBytes != code.bytes {
		t.Fatal("session switch changed bounded render cache accounting")
	}
}

func TestLargeMessageDisclosurePagesContentOnDemand(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	key := conversationCacheKey{sessionID: "session", itemID: "large-message", kind: desktopstate.TimelineAssistant}
	source := strings.Repeat("界", messagePageBytes*2)
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutLargeMessage(gtx, key, source, view.theme.Colors.OnSurface)
	router.Frame(gtx.Ops)
	buttons := view.conversationExpandButtons[key]
	if buttons == nil || view.conversationExpanded[key] {
		t.Fatal("large message did not start in bounded preview mode")
	}

	buttons.show.Click()
	operations.Reset()
	view.layoutLargeMessage(gtx, key, source, view.theme.Colors.OnSurface)
	router.Frame(gtx.Ops)
	if !view.conversationExpanded[key] || view.conversationPage[key] != 0 {
		t.Fatalf("show-full action state: expanded=%v page=%d", view.conversationExpanded[key], view.conversationPage[key])
	}

	buttons.next.Click()
	operations.Reset()
	view.layoutLargeMessage(gtx, key, source, view.theme.Colors.OnSurface)
	router.Frame(gtx.Ops)
	if page := view.conversationPage[key]; page != 1 {
		t.Fatalf("next-part action page = %d, want 1", page)
	}

	buttons.previous.Click()
	operations.Reset()
	view.layoutLargeMessage(gtx, key, source, view.theme.Colors.OnSurface)
	router.Frame(gtx.Ops)
	if page := view.conversationPage[key]; page != 0 {
		t.Fatalf("previous-part action page = %d, want 0", page)
	}

	buttons.collapse.Click()
	operations.Reset()
	view.layoutLargeMessage(gtx, key, source, view.theme.Colors.OnSurface)
	router.Frame(gtx.Ops)
	if view.conversationExpanded[key] {
		t.Fatal("show-less action did not return to the bounded preview")
	}
}

func TestLargeMessageDisclosureStateIsBounded(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	for index := range maxConversationExpansionKeys + 1 {
		key := conversationCacheKey{sessionID: "session", itemID: strconv.Itoa(index)}
		view.conversationExpanded[key] = true
		view.conversationPage[key] = index
		view.largeMessageDisclosureButtons(key)
	}
	if len(view.conversationExpandButtons) > maxConversationExpansionKeys || len(view.conversationExpanded) > maxConversationExpansionKeys || len(view.conversationPage) > maxConversationExpansionKeys {
		t.Fatalf("large-message UI state exceeded bound: buttons=%d expanded=%d pages=%d", len(view.conversationExpandButtons), len(view.conversationExpanded), len(view.conversationPage))
	}
}

func TestBuildSidebarRowsGroupsSessionsWithoutChangingOrder(t *testing.T) {
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{{ID: "p2", Name: "Second"}, {ID: "p1", Name: "First"}},
		Sessions: []desktopstate.SessionState{
			{ID: "s1", ProjectID: "p1", AgentID: controller.ProtonmanAgentID, Title: "One", Status: desktopstate.TaskIdle},
			{ID: "s2", ProjectID: "p2", AgentID: controller.ProtonmanAgentID, Title: "Two", Status: desktopstate.TaskRunning},
			{ID: "s3", ProjectID: "p1", AgentID: controller.ProtonmanAgentID, Title: "Three", Status: desktopstate.TaskCompleted},
			{ID: "orphan", ProjectID: "missing", AgentID: controller.ProtonmanAgentID, Title: "Hidden"},
		},
	}
	profiles := []app.ACPAgentProfile{controller.DefaultACPAgentProfile()}
	rows, cache := buildSidebarRows(state, profiles)
	want := []string{"p2", "s2", "p1", "s1", "s3"}
	if len(rows) != len(want) {
		t.Fatalf("sidebar rows = %#v, want ids %v", rows, want)
	}
	for index, id := range want {
		actual := rows[index].ProjectID
		if rows[index].Kind == sidebarSessionRow {
			actual = rows[index].SessionID
		}
		if actual != id {
			t.Fatalf("sidebar row %d id = %q, want %q", index, actual, id)
		}
	}
	if !cache.matches(state, profiles) {
		t.Fatal("sidebar cache did not match the state used to build it")
	}
	state.Sessions[2].Status = desktopstate.TaskFailed
	if cache.matches(state, profiles) {
		t.Fatal("sidebar cache matched after a visible session status changed")
	}
}

func TestMarkdownCacheStaysWithinByteBudget(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	oldKey := conversationCacheKey{sessionID: "old", itemID: "old"}
	key := conversationCacheKey{sessionID: "session", itemID: "new", kind: desktopstate.TimelineAssistant}
	view.conversationCache[oldKey] = conversationMarkdownCache{
		source: strings.Repeat("x", maxConversationCacheBytes),
		bytes:  maxConversationCacheBytes,
	}
	view.conversationCacheOrder = []conversationCacheKey{oldKey}
	view.conversationCacheBytes = maxConversationCacheBytes

	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(840, 600)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	source := "small **message**"
	view.layoutMarkdown(gtx, key, source, color.NRGBA{})

	cached := view.conversationCache[key]
	if len(view.conversationCache) != 1 || view.conversationCacheBytes != cached.bytes {
		t.Fatalf("cache after byte-budget eviction: entries=%d bytes=%d", len(view.conversationCache), view.conversationCacheBytes)
	}
	if cached.source != source {
		t.Fatalf("new markdown cache entry = %q, want %q", cached.source, source)
	}
}

func TestMarkdownCacheAccountsForExpandedSpanStorage(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	key := conversationCacheKey{sessionID: "session", itemID: "many-spans", kind: desktopstate.TimelineAssistant}
	source := strings.Repeat("**x** ", 20_000)
	spans, err := view.conversationMarkdown.Render([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	cached := conversationMarkdownCache{source: source, spans: spans}
	if size := markdownCacheEntryBytes(key, cached); size <= maxConversationCacheBytes {
		t.Fatalf("fixture cache weight = %d bytes across %d spans, want over %d", size, len(spans), maxConversationCacheBytes)
	}
	if !view.storeMarkdownCache(key, cached) {
		t.Fatal("plain-text fallback marker was not retained")
	}
	retained := view.conversationCache[key]
	if !retained.plain || len(retained.spans) != 0 || retained.bytes > maxConversationCacheBytes {
		t.Fatalf("high-expansion entry was not reduced to a bounded fallback: %#v", retained)
	}
	if len(view.conversationCache) != 1 || view.conversationCacheBytes != retained.bytes {
		t.Fatalf("fallback cache accounting: entries=%d bytes=%d want=%d", len(view.conversationCache), view.conversationCacheBytes, retained.bytes)
	}
}

func TestMarkdownCacheDropsStaleEntryForOversizedSource(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	key := conversationCacheKey{sessionID: "session", itemID: "growing", kind: desktopstate.TimelineAssistant}
	if !view.storeMarkdownCache(key, conversationMarkdownCache{source: "small", spans: []richtext.SpanStyle{{Content: "small"}}}) {
		t.Fatal("small markdown entry was not cached")
	}
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(840, 600)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutMarkdown(gtx, key, strings.Repeat("x", maxMarkdownRenderBytes+1), color.NRGBA{})
	if len(view.conversationCache) != 0 || view.conversationCacheBytes != 0 {
		t.Fatalf("stale markdown entry retained after source grew: entries=%d bytes=%d", len(view.conversationCache), view.conversationCacheBytes)
	}
}

func TestConversationItemDescriptionMatchesBoundedCompaction(t *testing.T) {
	items := []desktopstate.TimelineItem{
		{Kind: desktopstate.TimelineAssistant, Title: "Review", Status: "running", Text: "short answer"},
		{Kind: desktopstate.TimelineAssistant, Text: strings.Repeat("x", 1<<20)},
		{Kind: desktopstate.TimelineUser, Title: "  question  ", Text: strings.Repeat("界", 600)},
		{Kind: desktopstate.TimelineTool, Status: "", Text: "  output  "},
	}
	for _, item := range items {
		var parts []string
		parts = append(parts, string(item.Kind))
		if item.Title != "" {
			parts = append(parts, item.Title)
		}
		if item.Status != "" {
			parts = append(parts, item.Status)
		}
		if item.Text != "" {
			parts = append(parts, item.Text)
		}
		want := compactInspectorText(strings.Join(parts, ", "), 512)
		if got := conversationItemDescription(item); got != want {
			t.Fatalf("description = %q, want %q", got, want)
		}
	}
}

func TestMarkdownCacheBypassesOversizedMessages(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(840, 600)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	source := strings.Repeat("x", maxMarkdownRenderBytes+1)
	view.layoutMarkdown(gtx, conversationCacheKey{sessionID: "session", itemID: "large"}, source, color.NRGBA{})
	if len(view.conversationCache) != 0 || view.conversationCacheBytes != 0 {
		t.Fatalf("oversized markdown was cached: entries=%d bytes=%d", len(view.conversationCache), view.conversationCacheBytes)
	}
}

func TestResponseSplitCacheReusesStableSource(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	source := strings.Repeat("paragraph text with **markdown**\n\n", 64)
	key := conversationCacheKey{sessionID: "session", itemID: "assistant-1", kind: desktopstate.TimelineAssistant}

	first := view.responseBlocks(key, source)
	if len(view.conversationResponseCache) != 1 {
		t.Fatalf("response split was not cached: entries=%d", len(view.conversationResponseCache))
	}
	second := view.responseBlocks(key, source)
	if len(first) != len(second) || len(first) == 0 || first[0] != second[0] {
		t.Fatal("response split cache did not reuse the parsed blocks")
	}

	changed := strings.Repeat("different **content**\n\n", 64)
	view.responseBlocks(key, changed)
	if len(view.conversationResponseCache) != 1 {
		t.Fatalf("response split cache leaked entries: entries=%d", len(view.conversationResponseCache))
	}

	oversized := strings.Repeat("x", maxResponseSplitSourceBytes+1)
	view.responseBlocks(conversationCacheKey{sessionID: "session", itemID: "huge"}, oversized)
	if _, ok := view.conversationResponseCache[conversationCacheKey{sessionID: "session", itemID: "huge"}]; ok {
		t.Fatal("oversized response split was cached")
	}
}

func TestToolDiffInfoCachesClassificationBySource(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	diff := "diff --git a/foo/bar.go b/foo/bar.go\n--- a/foo/bar.go\n+++ b/foo/bar.go\n@@ -1 +1 @@\n-old\n+new\n"

	first := view.toolDiffInfo("tool-1", diff)
	if !first.isDiff || first.filename != "foo/bar.go" || first.additions != 1 || first.deletions != 1 || len(first.preview) == 0 {
		t.Fatalf("diff classification = %+v", first)
	}

	plain := view.toolDiffInfo("tool-1", "plain output")
	if plain.isDiff {
		t.Fatalf("plain output classified as diff: %+v", plain)
	}

	// The stored entry tracks the latest source for the key.
	if cached, ok := view.toolDiffCache.Get("tool-1"); !ok || cached.source != "plain output" || cached.isDiff {
		t.Fatalf("cached entry = %+v", cached)
	}

	// Repeated lookups for an unchanged source return the cached entry.
	if got := view.toolDiffInfo("tool-1", "plain output"); got.source != "plain output" {
		t.Fatalf("cached lookup = %+v", got)
	}
}

func TestToolDiffInfoCacheIsBounded(t *testing.T) {
	view := New(NewTheme("light"), Bindings{})
	for index := 0; index <= maxToolDiffCacheEntries; index++ {
		view.toolDiffInfo("tool-"+strconv.Itoa(index), "output")
	}
	if got := view.toolDiffCache.Len(); got > maxToolDiffCacheEntries {
		t.Fatalf("tool diff cache = %d entries, want <= %d", got, maxToolDiffCacheEntries)
	}
}

func TestParsedThinkingCachesBySource(t *testing.T) {
	view := New(NewTheme("dark"), Bindings{})
	key := conversationCacheKey{sessionID: "session", itemID: "assistant-1", kind: desktopstate.TimelineAssistant}
	text := "<think>\nreasoning about it\n</think>\nfinal answer"

	first := view.parsedThinking(key, text)
	if !first.hasThinking || !first.thinkingDone || first.responseText != "final answer" {
		t.Fatalf("parsed thinking = %+v", first)
	}
	if cached, ok := view.conversationThinkingCache.Get(key); !ok || cached.source != text {
		t.Fatalf("thinking parse was not cached: %#v", view.conversationThinkingCache)
	}

	// A changed source for the same key must be recomputed, not served stale.
	second := view.parsedThinking(key, "plain reply")
	if second.hasThinking || second.responseText != "plain reply" {
		t.Fatalf("recomputed parse = %+v", second)
	}
	if cached, ok := view.conversationThinkingCache.Get(key); !ok || cached.source != "plain reply" {
		t.Fatalf("thinking cache did not track new source: %#v", cached)
	}
}
