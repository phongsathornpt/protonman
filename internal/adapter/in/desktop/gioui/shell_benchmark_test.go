//go:build desktop || desktop_gio

package gioui

import (
	"image"
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
		snapshot.State.Sessions[0].Timeline[0].Text += "x"
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
	snapshot.State.Sessions[0].Timeline = []desktopstate.TimelineItem{{
		Kind:      desktopstate.TimelineAssistant,
		ID:        "assistant-1",
		Text:      strings.Repeat("streamed markdown **content** ", 640),
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
		snapshot.State.Sessions[0].Timeline[0].Text += "x"
		snapshot.Revision++
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
