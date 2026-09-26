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
