//go:build desktop || desktop_gio

package gioui

import (
	"image"
	"strconv"
	"strings"
	"testing"
	"time"

	"gioui.org/font"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

var compactTextBenchmarkSink string

func BenchmarkCompactInspectorTextLongMessage(b *testing.B) {
	message := strings.Repeat("長いストリーミング応答です。", 1<<15)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		compactTextBenchmarkSink = compactInspectorText(message, 512)
	}
}

func BenchmarkConversationItemDescriptionBounded(b *testing.B) {
	item := desktopstate.TimelineItem{
		Kind: desktopstate.TimelineAssistant,
		Text: strings.Repeat("x", maxMessageStreamBytes),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		compactTextBenchmarkSink = conversationItemDescription(item)
	}
}

func BenchmarkConversationItemDescriptionLegacy(b *testing.B) {
	item := desktopstate.TimelineItem{
		Kind: desktopstate.TimelineAssistant,
		Text: strings.Repeat("x", maxMessageStreamBytes),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		parts := []string{string(item.Kind)}
		if item.Title != "" {
			parts = append(parts, item.Title)
		}
		if item.Status != "" {
			parts = append(parts, item.Status)
		}
		if item.Text != "" {
			parts = append(parts, item.Text)
		}
		compactTextBenchmarkSink = compactInspectorText(strings.Join(parts, ", "), 512)
	}
}

func BenchmarkShellLayoutStable(b *testing.B) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
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

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		operations.Reset()
		view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
	}
}

func BenchmarkShellLayoutLargeHistory(b *testing.B) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
	const count = 8192
	timeline := make([]desktopstate.TimelineItem, count)
	for index := range timeline {
		timeline[index] = desktopstate.TimelineItem{
			Kind: desktopstate.TimelineUser,
			ID:   "message-" + strconv.Itoa(index),
			Text: "A retained message in a large session.",
		}
	}
	snapshot.State.Sessions[0].Timeline = timeline
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

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		operations.Reset()
		view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
	}
}

func BenchmarkUpdateMentionStateStableLargeDraft(b *testing.B) {
	view := newShell(newTheme("light"))
	draft := strings.Repeat("a", 64<<10-4) + " @str"
	view.setComposerText(draft)
	view.composer.SetCaret(view.composer.Len(), view.composer.Len())
	view.updateMentionState("")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		view.updateMentionState("")
	}
}

func BenchmarkRefreshMentionStateStableLargeDraft(b *testing.B) {
	view := newShell(newTheme("light"))
	draft := strings.Repeat("a", 64<<10-4) + " @str"
	view.setComposerText(draft)
	view.composer.SetCaret(view.composer.Len(), view.composer.Len())
	view.refreshMentionState("")

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		view.refreshMentionState("")
	}
}

func BenchmarkSidebarRowsCacheMatchesManyProjects(b *testing.B) {
	const count = 256
	state := desktopstate.State{
		Projects: make([]desktopstate.ProjectState, count),
		Sessions: make([]desktopstate.SessionState, count),
	}
	for index := range count {
		id := strconv.Itoa(index)
		projectID := "project-" + id
		state.Projects[index] = desktopstate.ProjectState{ID: projectID, Name: "Project " + id}
		state.Sessions[index] = desktopstate.SessionState{
			ID:        "session-" + id,
			ProjectID: projectID,
			AgentID:   controllerAgentID,
			Title:     "Session " + id,
			Status:    desktopstate.TaskIdle,
		}
	}
	profiles := []app.ACPAgentProfile{defaultACPAgentProfile()}
	_, cache := buildSidebarRows(state, profiles)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if !cache.matches(state, profiles) {
			b.Fatal("unchanged sidebar state did not match cache")
		}
	}
}

func BenchmarkShellLayoutUncachedSidebarRows(b *testing.B) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
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

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		view.sidebarRowsCache.valid = false
		operations.Reset()
		view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
	}
}

func BenchmarkShellLayoutStreamingAssistant(b *testing.B) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
	textVariants := benchmarkStreamingTextVariants(strings.Repeat("streamed markdown **content** ", 160))
	snapshot.State.Sessions[0].Timeline = []desktopstate.TimelineItem{{
		Kind:      desktopstate.TimelineAssistant,
		ID:        "assistant-1",
		Text:      textVariants[0],
		Streaming: true,
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

	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		snapshot.State.Sessions[0].Timeline[0].Text = textVariants[index%len(textVariants)]
		snapshot.Revision++
		operations.Reset()
		view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
	}
}

func BenchmarkShellLayoutStreamingAssistantStableText(b *testing.B) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
	snapshot.State.Sessions[0].Timeline = []desktopstate.TimelineItem{{
		Kind:      desktopstate.TimelineAssistant,
		ID:        "assistant-1",
		Text:      strings.Repeat("streamed markdown **content** ", 160),
		Streaming: true,
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

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		snapshot.Revision++
		operations.Reset()
		view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
	}
}

func BenchmarkShellLayoutStreamingAssistantLongText(b *testing.B) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
	textVariants := benchmarkStreamingTextVariants(strings.Repeat("streamed markdown **content** ", 640))
	snapshot.State.Sessions[0].Timeline = []desktopstate.TimelineItem{{
		Kind:      desktopstate.TimelineAssistant,
		ID:        "assistant-1",
		Text:      textVariants[0],
		Streaming: true,
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

	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		snapshot.State.Sessions[0].Timeline[0].Text = textVariants[index%len(textVariants)]
		snapshot.Revision++
		operations.Reset()
		view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
	}
}

func BenchmarkShellLayoutCompletedAssistantOversized(b *testing.B) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
	textA := strings.Repeat("completed response **markdown** a ", 1<<15)
	textB := strings.Repeat("completed response **markdown** b ", 1<<15)
	snapshot.State.Sessions[0].Timeline = []desktopstate.TimelineItem{{
		Kind: desktopstate.TimelineAssistant,
		ID:   "assistant-completed",
		Text: textA,
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

	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		if index%2 == 0 {
			snapshot.State.Sessions[0].Timeline[0].Text = textB
		} else {
			snapshot.State.Sessions[0].Timeline[0].Text = textA
		}
		snapshot.Revision++
		operations.Reset()
		view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
	}
}

func BenchmarkShellLayoutExpandedAssistantOversizedPart(b *testing.B) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
	textA := strings.Repeat("completed response **markdown** a ", 1<<15)
	textB := strings.Repeat("completed response **markdown** b ", 1<<15)
	snapshot.State.Sessions[0].Timeline = []desktopstate.TimelineItem{{
		Kind: desktopstate.TimelineAssistant,
		ID:   "assistant-completed",
		Text: textA,
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
	view.conversationExpanded[key] = true
	view.conversationPage[key] = 12
	view.layout(gtx, snapshot)
	router.Frame(gtx.Ops)

	b.ReportAllocs()
	b.ResetTimer()
	for index := range b.N {
		if index%2 == 0 {
			snapshot.State.Sessions[0].Timeline[0].Text = textB
		} else {
			snapshot.State.Sessions[0].Timeline[0].Text = textA
		}
		snapshot.Revision++
		operations.Reset()
		view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
	}
}

func benchmarkStreamingTextVariants(text string) []string {
	prefix := text[:len(text)-1]
	variants := make([]string, 26)
	for index := range variants {
		variants[index] = prefix + string(rune('a'+index))
	}
	return variants
}

func BenchmarkShellLayoutLargeHistoryRichResponses(b *testing.B) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
	const count = 8192
	prose := strings.Repeat("streamed markdown **content** with a [link](https://example.com) and `inline code`.\n\n", 16)
	timeline := make([]desktopstate.TimelineItem, count)
	for index := range timeline {
		kind := desktopstate.TimelineAssistant
		text := prose
		if index%2 == 0 {
			kind = desktopstate.TimelineUser
			text = "A retained user message in a large session."
		}
		timeline[index] = desktopstate.TimelineItem{
			Kind: kind,
			ID:   "message-" + strconv.Itoa(index),
			Text: text,
		}
	}
	snapshot.State.Sessions[0].Timeline = timeline
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

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		operations.Reset()
		view.layout(gtx, snapshot)
		router.Frame(gtx.Ops)
	}
}

func BenchmarkLayoutLabelLongText(b *testing.B) {
	view := newShell(newTheme("light"))
	text := strings.Repeat("streamed markdown **content** ", 160)
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(840, 600)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	view.layoutLabel(gtx, text, textBodyMedium, font.Normal, view.theme.onSurface, 0)
	router.Frame(gtx.Ops)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		operations.Reset()
		view.layoutLabel(gtx, text, textBodyMedium, font.Normal, view.theme.onSurface, 0)
		router.Frame(gtx.Ops)
	}
}

// benchmarkToolTimeline builds a conversation timeline dominated by expanded
// tool items, the scroll-jank worst case: half the items are diffs (which
// previously defaulted expanded) and half carry large plain tool output.
func benchmarkToolTimeline(count int) []desktopstate.TimelineItem {
	timeline := make([]desktopstate.TimelineItem, count)
	for index := range timeline {
		if index%2 == 0 {
			lines := make([]string, 0, 200)
			for line := 0; line < 200; line++ {
				marker := "+"
				if line%2 == 0 {
					marker = "-"
				}
				lines = append(lines, marker+" changed line of the patch body "+strconv.Itoa(line))
			}
			timeline[index] = desktopstate.TimelineItem{
				Kind:   desktopstate.TimelineTool,
				ID:     "tool-diff-" + strconv.Itoa(index),
				Title:  "edit",
				Status: "completed",
				Text:   "diff --git a/file.go b/file.go\n@@ -1,2 +1,2 @@\n" + strings.Join(lines, "\n"),
			}
			continue
		}
		timeline[index] = desktopstate.TimelineItem{
			Kind:   desktopstate.TimelineTool,
			ID:     "tool-out-" + strconv.Itoa(index),
			Title:  "bash",
			Status: "completed",
			Text:   strings.Repeat("tool output line with some diagnostic detail\n", 2000),
		}
	}
	return timeline
}

// benchmarkConversationFrame lays out the conversation list once against a
// fixed frame context, mirroring how production draws a single frame.
func benchmarkConversationFrame(b *testing.B, timeline []desktopstate.TimelineItem) {
	view := newShell(newTheme("light"))
	snapshot := benchmarkShellSnapshot()
	snapshot.State.Sessions[0].Timeline = timeline
	var operations op.Ops
	var router input.Router
	gtx := layout.Context{
		Ops:         &operations,
		Constraints: layout.Exact(image.Pt(1180, 760)),
		Metric:      unit.Metric{},
		Now:         time.Unix(1, 0),
		Source:      router.Source(),
	}
	session := snapshot.State.Sessions[0]
	view.layoutConversation(gtx, session, historyStateLoaded)
	router.Frame(gtx.Ops)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		operations.Reset()
		view.layoutConversation(gtx, session, historyStateLoaded)
		router.Frame(gtx.Ops)
	}
}

func BenchmarkConversationFrameToolHeavy(b *testing.B) {
	benchmarkConversationFrame(b, benchmarkToolTimeline(64))
}

// BenchmarkConversationFrameMixedProse renders a transcript shaped like a real
// working session: prose answers with code cards, one oversized paged answer,
// and interleaved user messages.
func BenchmarkConversationFrameMixedProse(b *testing.B) {
	prose := "## Plan\n\nRun the checks first.\n\n```go\nfunc main() {\n\tfmt.Println(\"hello world\")\n}\n```\n\nDone. See the numbers above and the referenced file list."
	timeline := make([]desktopstate.TimelineItem, 48)
	for index := range timeline {
		switch index % 4 {
		case 0:
			timeline[index] = desktopstate.TimelineItem{
				Kind: desktopstate.TimelineUser,
				ID:   "message-" + strconv.Itoa(index),
				Text: "Please continue with step " + strconv.Itoa(index) + " and verify the result.",
			}
		case 1:
			timeline[index] = desktopstate.TimelineItem{
				Kind:      desktopstate.TimelineAssistant,
				ID:        "message-" + strconv.Itoa(index),
				Text:      prose,
				Streaming: false,
			}
		case 2:
			timeline[index] = desktopstate.TimelineItem{
				Kind:      desktopstate.TimelineAssistant,
				ID:        "message-" + strconv.Itoa(index),
				Text:      strings.Repeat("long analysis paragraph with markdown **emphasis** and `inline code`.\n\n", 1500),
				Streaming: false,
			}
		default:
			timeline[index] = desktopstate.TimelineItem{
				Kind:  desktopstate.TimelineTool,
				ID:    "tool-" + strconv.Itoa(index),
				Title: "bash",
				Status: "completed",
				Text:  "exit 0\n" + strings.Repeat("output line\n", 60),
			}
		}
	}
	benchmarkConversationFrame(b, timeline)
}

func BenchmarkConversationFrameToolHeavyScaling(b *testing.B) {
	for _, items := range []int{16, 64, 256} {
		b.Run("items-"+strconv.Itoa(items), func(b *testing.B) {
			benchmarkConversationFrame(b, benchmarkToolTimeline(items))
		})
	}
}

func benchmarkShellSnapshot() controllerSnapshot {
	sessions := make([]desktopstate.SessionState, 32)
	for index := range sessions {
		sessions[index] = desktopstate.SessionState{
			ID:        "session-" + string(rune('a'+index)),
			AgentID:   controllerAgentID,
			ProjectID: "project",
			Status:    desktopstate.TaskRunning,
			Timeline:  make([]desktopstate.TimelineItem, 64),
		}
	}
	return controllerSnapshot{
		State: desktopstate.State{
			ActiveSessionID: sessions[0].ID,
			ActiveProjectID: "project",
			Projects:        []desktopstate.ProjectState{{ID: "project", Name: "project"}},
			Sessions:        sessions,
		},
		AgentProfiles: []app.ACPAgentProfile{defaultACPAgentProfile()},
		Connection:    connectionConnected,
		Status:        "Connected",
		Revision:      1,
	}
}
