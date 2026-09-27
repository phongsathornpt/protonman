//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"image/color"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/x/richtext"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestDesktopStreamingRenderRetentionSoak(t *testing.T) {
	view := newShell(newTheme("light"))
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
	view.layout(gtx, snapshot)
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
			view.layout(gtx, snapshot)
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
	view := newShell(newTheme("light"))
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
	view.layout(gtx, snapshot)
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
			view.layout(gtx, snapshot)
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
	source := strings.Repeat("界", largeMessagePreviewBytes)
	preview := largeMessagePreview(source)
	if !utf8.ValidString(preview) {
		t.Fatal("large message preview contains invalid UTF-8")
	}
	if len(preview) > largeMessagePreviewBytes+len("\n…") {
		t.Fatalf("preview bytes = %d, limit = %d", len(preview), largeMessagePreviewBytes+len("\n…"))
	}
	if !strings.HasSuffix(preview, "\n…") {
		t.Fatalf("large message preview lacks its truncation marker: %q", preview[len(preview)-8:])
	}
	if got := largeMessagePreview("short message"); got != "short message" {
		t.Fatalf("short preview = %q", got)
	}

	windowSource := strings.Repeat("界", largeMessagePageBytes*2) + " tail"
	pageCount := (len(windowSource) + largeMessagePageBytes - 1) / largeMessagePageBytes
	var reconstructed strings.Builder
	previousEnd := 0
	for page := range pageCount {
		window, start, end := largeMessageWindow(windowSource, page)
		if start != previousEnd || end < start || end-start > largeMessagePageBytes+utf8.UTFMax {
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
	view := newShell(newTheme("light"))
	key := conversationCacheKey{sessionID: "old-session", itemID: "large-item"}
	view.conversationExpanded[key] = true
	view.conversationPage[key] = 2
	view.conversationExpandButtons[key] = &conversationDisclosureButtons{}

	view.syncConversation(desktopstate.State{ActiveSessionID: "new-session"})
	if len(view.conversationExpanded) != 0 || len(view.conversationPage) != 0 || len(view.conversationExpandButtons) != 0 {
		t.Fatalf("session switch retained large-message disclosure state: expanded=%d pages=%d buttons=%d", len(view.conversationExpanded), len(view.conversationPage), len(view.conversationExpandButtons))
	}
}

func TestLargeMessageDisclosurePagesContentOnDemand(t *testing.T) {
	view := newShell(newTheme("light"))
	key := conversationCacheKey{sessionID: "session", itemID: "large-message", kind: desktopstate.TimelineAssistant}
	source := strings.Repeat("界", largeMessagePageBytes*2)
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(800, 600)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutLargeMessage(gtx, key, source, view.theme.onSurface)
	router.Frame(gtx.Ops)
	buttons := view.conversationExpandButtons[key]
	if buttons == nil || view.conversationExpanded[key] {
		t.Fatal("large message did not start in bounded preview mode")
	}

	buttons.show.Click()
	operations.Reset()
	view.layoutLargeMessage(gtx, key, source, view.theme.onSurface)
	router.Frame(gtx.Ops)
	if !view.conversationExpanded[key] || view.conversationPage[key] != 0 {
		t.Fatalf("show-full action state: expanded=%v page=%d", view.conversationExpanded[key], view.conversationPage[key])
	}

	buttons.next.Click()
	operations.Reset()
	view.layoutLargeMessage(gtx, key, source, view.theme.onSurface)
	router.Frame(gtx.Ops)
	if page := view.conversationPage[key]; page != 1 {
		t.Fatalf("next-part action page = %d, want 1", page)
	}

	buttons.previous.Click()
	operations.Reset()
	view.layoutLargeMessage(gtx, key, source, view.theme.onSurface)
	router.Frame(gtx.Ops)
	if page := view.conversationPage[key]; page != 0 {
		t.Fatalf("previous-part action page = %d, want 0", page)
	}

	buttons.collapse.Click()
	operations.Reset()
	view.layoutLargeMessage(gtx, key, source, view.theme.onSurface)
	router.Frame(gtx.Ops)
	if view.conversationExpanded[key] {
		t.Fatal("show-less action did not return to the bounded preview")
	}
}

func TestLargeMessageDisclosureStateIsBounded(t *testing.T) {
	view := newShell(newTheme("light"))
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

func TestShellLaysOutResponsiveStates(t *testing.T) {
	view := newShell(newTheme("light"))
	if view.sidebarList.Axis != layout.Vertical || view.conversationList.Axis != layout.Vertical {
		t.Fatalf("lists must scroll vertically: sidebar=%v conversation=%v", view.sidebarList.Axis, view.conversationList.Axis)
	}
	if !view.conversationList.ScrollToEnd {
		t.Fatal("conversation list must follow new timeline items")
	}
	snapshots := []controllerSnapshot{
		{State: desktopstate.State{}, Connection: connectionConnecting, Status: "Starting Protonman…"},
		{
			State: desktopstate.State{
				ActiveSessionID: "session",
				ActiveProjectID: "workspace:/workspace/alpha",
				Projects: []desktopstate.ProjectState{{
					ID:   "workspace:/workspace/alpha",
					Name: "alpha",
				}},
				Sessions: []desktopstate.SessionState{{
					ID:            "session",
					ProjectID:     "workspace:/workspace/alpha",
					Title:         "Long session title that must remain bounded",
					Workspace:     "/workspace/alpha",
					WorkspaceKey:  "workspace:/workspace/alpha",
					WorkspaceName: "alpha",
					AgentID:       controllerAgentID,
					Status:        desktopstate.TaskIdle,
				}},
			},
			Connection: connectionConnected,
			Status:     "Connected",
		},
		{
			State: desktopstate.State{
				ActiveSessionID: "session",
				ActiveProjectID: "workspace:/workspace/alpha",
				Projects: []desktopstate.ProjectState{{
					ID:   "workspace:/workspace/alpha",
					Name: "alpha",
				}},
				Sessions: []desktopstate.SessionState{{
					ID:            "session",
					ProjectID:     "workspace:/workspace/alpha",
					Title:         "Conversation",
					Workspace:     "/workspace/alpha",
					WorkspaceKey:  "workspace:/workspace/alpha",
					WorkspaceName: "alpha",
					AgentID:       controllerAgentID,
					Status:        desktopstate.TaskWaitingPermission,
					Timeline: []desktopstate.TimelineItem{
						{ID: "user-1", Kind: desktopstate.TimelineUser, Text: "Review the change"},
						{ID: "assistant-1", Kind: desktopstate.TimelineAssistant, Text: "**Done**"},
						{ID: "tool-1", Kind: desktopstate.TimelineTool, Title: "Read file", Status: "completed", Text: "ok"},
					},
					Subagents: []desktopstate.SubagentState{{ID: "agent-1", Profile: "strength", Task: "Inspect tests", Status: "running"}},
				}},
				PermissionInbox: []desktopstate.PermissionRequest{{
					RequestID: "permission-1",
					SessionID: "session",
					Title:     "Run tests",
					Detail:    "Tool request:\nrun tests",
					Options: []desktopstate.PermissionOption{
						{ID: "allow", Name: "Allow once"},
						{ID: "reject", Name: "Reject"},
					},
				}},
			},
			Connection: connectionConnected,
			Status:     "Permission required",
		},
	}
	view.composer.SetText("follow-up")
	sizes := []image.Point{{X: 760, Y: 600}, {X: 1180, Y: 760}}
	for _, snapshot := range snapshots {
		for _, size := range sizes {
			var operations op.Ops
			var router input.Router
			gtx := layout.Context{
				Ops:         &operations,
				Constraints: layout.Exact(size),
				Metric:      unit.Metric{},
				Now:         time.Unix(1, 0),
				Source:      router.Source(),
			}
			dims := view.layout(gtx, snapshot)
			router.Frame(gtx.Ops)
			if dims.Size != size {
				t.Fatalf("layout size = %v, want %v", dims.Size, size)
			}
		}
	}
}

func TestBuildSidebarRowsGroupsSessionsWithoutChangingOrder(t *testing.T) {
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{{ID: "p2", Name: "Second"}, {ID: "p1", Name: "First"}},
		Sessions: []desktopstate.SessionState{
			{ID: "s1", ProjectID: "p1", AgentID: controllerAgentID, Title: "One", Status: desktopstate.TaskIdle},
			{ID: "s2", ProjectID: "p2", AgentID: controllerAgentID, Title: "Two", Status: desktopstate.TaskRunning},
			{ID: "s3", ProjectID: "p1", AgentID: controllerAgentID, Title: "Three", Status: desktopstate.TaskCompleted},
			{ID: "orphan", ProjectID: "missing", AgentID: controllerAgentID, Title: "Hidden"},
		},
	}
	profiles := []app.ACPAgentProfile{defaultACPAgentProfile()}
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

func TestBuildSidebarRowsPutsRecentSessionsFirstAndHidesIdleBadges(t *testing.T) {
	activity := time.Date(2026, 9, 26, 8, 0, 0, 0, time.UTC)
	state := desktopstate.State{
		Projects: []desktopstate.ProjectState{{ID: "project", Name: "Project"}},
		Sessions: []desktopstate.SessionState{
			{ID: "older", ProjectID: "project", AgentID: controllerAgentID, Title: "hi", Status: desktopstate.TaskIdle, LastActivityAt: activity},
			{ID: "newer", ProjectID: "project", AgentID: controllerAgentID, Title: "hi", Status: desktopstate.TaskIdle, LastActivityAt: activity.Add(time.Hour)},
			{ID: "unknown", ProjectID: "project", AgentID: controllerAgentID, Title: "proton", Status: desktopstate.TaskIdle},
		},
	}
	rows, _ := buildSidebarRows(state, []app.ACPAgentProfile{defaultACPAgentProfile()})
	if len(rows) != 4 || rows[0].SessionCount != 3 {
		t.Fatalf("rows = %#v, want project count 3 and three sessions", rows)
	}
	if rows[1].SessionID != "newer" || rows[2].SessionID != "older" || rows[3].SessionID != "unknown" {
		t.Fatalf("session order = %q, %q, %q", rows[1].SessionID, rows[2].SessionID, rows[3].SessionID)
	}
	if got := sidebarStatusLabel("idle"); got != "" {
		t.Fatalf("idle status label = %q, want hidden", got)
	}
	if got := sidebarStatusLabel("waiting_permission"); got != "Approval" {
		t.Fatalf("permission status label = %q, want Approval", got)
	}
	now := activity.Add(3 * time.Minute)
	if got := sidebarSessionSubtitle(rows[2], now); got != "3m ago" {
		t.Fatalf("activity subtitle = %q, want 3m ago", got)
	}
	if got := sidebarSessionSubtitle(rows[3], now); got != "" {
		t.Fatalf("default-agent fallback subtitle = %q, want empty", got)
	}
	state.Sessions[0].LastActivityAt = activity.Add(2 * time.Hour)
	cache := sidebarRowsCache{valid: true}
	_, cache = buildSidebarRows(state, []app.ACPAgentProfile{defaultACPAgentProfile()})
	state.Sessions[0].LastActivityAt = activity.Add(4 * time.Hour)
	if cache.matches(state, []app.ACPAgentProfile{defaultACPAgentProfile()}) {
		t.Fatal("sidebar cache matched after a session activity timestamp changed")
	}
}

func TestMarkdownCacheStaysWithinByteBudget(t *testing.T) {
	view := newShell(newTheme("light"))
	key := conversationCacheKey{sessionID: "session", itemID: "new", kind: desktopstate.TimelineAssistant}
	view.conversationCache[conversationCacheKey{sessionID: "old", itemID: "old"}] = conversationMarkdownCache{
		source: strings.Repeat("x", maxConversationCacheBytes),
		bytes:  maxConversationCacheBytes,
	}
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
	view := newShell(newTheme("light"))
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
	view := newShell(newTheme("light"))
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
	view.layoutMarkdown(gtx, key, strings.Repeat("x", maxCachedMarkdownItemBytes+1), color.NRGBA{})
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
	view := newShell(newTheme("light"))
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(840, 600)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	source := strings.Repeat("x", maxCachedMarkdownItemBytes+1)
	view.layoutMarkdown(gtx, conversationCacheKey{sessionID: "session", itemID: "large"}, source, color.NRGBA{})
	if len(view.conversationCache) != 0 || view.conversationCacheBytes != 0 {
		t.Fatalf("oversized markdown was cached: entries=%d bytes=%d", len(view.conversationCache), view.conversationCacheBytes)
	}
}

func TestShellLaysOutInspectorContentAtBothBreakpoints(t *testing.T) {
	view := newShell(newTheme("dark"))
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			ActiveSessionID: "session",
			Sessions: []desktopstate.SessionState{{
				ID:     "session",
				Title:  "Inspector",
				Status: desktopstate.TaskIdle,
				Context: desktopstate.SessionContextState{
					Goal: "Ship the inspector slice",
					Todo: desktopstate.TodoState{Revision: 7, Items: []desktopstate.TodoItemState{{ID: "todo", Text: "Run focused tests", Status: "in_progress"}}},
					Memory: desktopstate.MemoryState{
						WorkspaceKey: "workspace-1",
						Workspace:    []desktopstate.MemoryEntryState{{ID: "m1", Kind: "repo_fact", Key: "test command", Value: "go test ./...", Confidence: .9, UsageCount: 2}},
						Global:       []desktopstate.MemoryEntryState{{ID: "m2", Kind: "preference", Key: "style", Value: "concise", Confidence: 1}},
					},
				},
				Runtime: desktopstate.RuntimeSettingsState{Provider: "openai", Model: "gpt", Reasoning: "high", LowConcurrency: "on"},
			}},
		},
		Connection: connectionConnected,
		Status:     "Connected",
	}
	view.inspectorOverride = true
	view.inspectorVisible = true
	for _, size := range []image.Point{{X: 760, Y: 600}, {X: 1180, Y: 760}} {
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(size),
			Metric:      unit.Metric{},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
		if dims.Size != size {
			t.Fatalf("layout size = %v, want %v", dims.Size, size)
		}
	}
}

func TestSyncAgentProfileEditorsPreservesUserDraft(t *testing.T) {
	view := newShell(newTheme("light"))
	profile := app.ACPAgentProfile{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer-acp", Args: []string{"--stdio"}, Env: []string{"TOKEN"}}
	snapshot := controllerSnapshot{AgentProfiles: []app.ACPAgentProfile{profile}}
	view.agentEditorVisible = true
	view.agentEditorOriginalID = profile.ID
	view.syncAgentProfileEditors(snapshot)
	if view.agentCommandEditor.Text() != profile.Command || view.agentEnvEditor.Text() != `["TOKEN"]` {
		t.Fatalf("editor was not populated: command=%q env=%q", view.agentCommandEditor.Text(), view.agentEnvEditor.Text())
	}
	view.agentCommandEditor.SetText("draft-command")
	view.syncAgentProfileEditors(snapshot)
	if view.agentCommandEditor.Text() != "draft-command" {
		t.Fatalf("user draft was overwritten: %q", view.agentCommandEditor.Text())
	}
}

func TestShellLaysOutExternalAgentAndProfileEditor(t *testing.T) {
	view := newShell(newTheme("dark"))
	view.agentSelectorVisible = true
	view.inspectorOverride = true
	view.inspectorVisible = true
	view.agentEditorVisible = true
	view.agentEditorOriginalID = "reviewer"
	profiles := []app.ACPAgentProfile{
		{ID: controllerAgentID, DisplayName: "Protonman", Command: "protonman", Args: []string{"--acp"}},
		{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer-acp", Args: []string{"--stdio"}, Env: []string{"TOKEN"}},
	}
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			ActiveSessionID: "review-session",
			ActiveProjectID: "project",
			Projects:        []desktopstate.ProjectState{{ID: "project", Name: "project", DefaultAgentID: "reviewer", AgentIDs: []string{"reviewer"}}},
			Sessions: []desktopstate.SessionState{{
				ID: "review-session", ProjectID: "project", Title: "Review", AgentID: "reviewer", Status: desktopstate.TaskIdle,
			}},
		},
		ActiveAgentID:    "reviewer",
		AgentProfiles:    profiles,
		AgentConnections: map[string]connectionPhase{controllerAgentID: connectionConnected, "reviewer": connectionConnected},
		Connection:       connectionConnected,
		Status:           "Connected",
	}
	view.syncAgentProfileEditors(snapshot)
	for _, size := range []image.Point{{X: 760, Y: 600}, {X: 1180, Y: 760}} {
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(size),
			Metric:      unit.Metric{},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
		if dims.Size != size {
			t.Fatalf("layout size = %v, want %v", dims.Size, size)
		}
	}
}

func TestShellLaysOutMCPIntegrationFormAtBothBreakpoints(t *testing.T) {
	view := newShell(newTheme("light"))
	view.inspectorOverride = true
	view.inspectorVisible = true
	view.mcpFormVisible = true
	view.mcpSelectedName = "docs"
	snapshot := controllerSnapshot{
		State: desktopstate.State{
			ActiveSessionID: "session",
			Sessions: []desktopstate.SessionState{{
				ID:     "session",
				Title:  "MCP",
				Status: desktopstate.TaskIdle,
			}},
			Integrations: []desktopstate.MCPIntegrationState{{
				Name: "docs", Command: "mcp-docs", Args: []string{"--stdio"}, Env: []string{"TOKEN"},
			}},
		},
		Connection: connectionConnected,
		Status:     "Connected",
	}
	for _, size := range []image.Point{{X: 760, Y: 600}, {X: 1180, Y: 760}} {
		var operations op.Ops
		var router input.Router
		gtx := layout.Context{
			Ops:         &operations,
			Constraints: layout.Exact(size),
			Metric:      unit.Metric{},
			Now:         time.Unix(1, 0),
			Source:      router.Source(),
		}
		dims := view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
		if dims.Size != size {
			t.Fatalf("layout size = %v, want %v", dims.Size, size)
		}
	}
}
