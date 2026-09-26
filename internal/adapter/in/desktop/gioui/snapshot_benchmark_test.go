//go:build desktop || desktop_gio

package gioui

import (
	"strconv"
	"strings"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func BenchmarkControllerSnapshotCached(b *testing.B) {
	controller := benchmarkController()
	controller.snapshot()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = controller.snapshot()
	}
}

func BenchmarkControllerSnapshotAfterRevision(b *testing.B) {
	controller := benchmarkController()
	controller.snapshot()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		controller.revision++
		_ = controller.snapshot()
	}
}

func BenchmarkControllerSnapshotRetentionLimit(b *testing.B) {
	controller := benchmarkController()
	timeline := make([]desktopstate.TimelineItem, maxSessionTimelineItems)
	text := strings.Repeat("x", 900)
	for index := range timeline {
		timeline[index] = desktopstate.TimelineItem{
			Kind: desktopstate.TimelineAssistant,
			ID:   "message-" + strconv.Itoa(index),
			Text: text,
		}
	}
	controller.state.Sessions[0].Timeline = timeline
	controller.snapshot()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		controller.revision++
		_ = controller.snapshot()
	}
}

func BenchmarkControllerSnapshotAfterTimelineRevision(b *testing.B) {
	controller := benchmarkController()
	timeline := make([]desktopstate.TimelineItem, maxSessionTimelineItems)
	text := strings.Repeat("x", 900)
	for index := range timeline {
		timeline[index] = desktopstate.TimelineItem{
			Kind: desktopstate.TimelineAssistant,
			ID:   "message-" + strconv.Itoa(index),
			Text: text,
		}
	}
	controller.state.Sessions[0].Timeline = timeline
	controller.snapshot()
	nextText := text + "x"

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		controller.mu.Lock()
		controller.state.Sessions[0].Timeline[len(timeline)-1].Text = nextText
		controller.revision++
		if !controller.advanceSnapshotCacheForTimelineLocked(controller.state.ActiveSessionID) {
			controller.snapshotCache = controllerSnapshotCache{}
		}
		controller.mu.Unlock()
		_ = controller.snapshot()
	}
}

func BenchmarkControllerSnapshotAfterStreamRevision(b *testing.B) {
	controller := benchmarkController()
	controller.state.Sessions[0].Timeline[0] = desktopstate.TimelineItem{
		Kind: desktopstate.TimelineAssistant,
		ID:   "message-0",
		Text: "before",
	}
	controller.snapshot()
	nextText := "after"

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		controller.mu.Lock()
		controller.state.Sessions[0].Timeline[0].Text = nextText
		controller.revision++
		if !controller.advanceSnapshotCacheForTimelineLocked(controller.state.ActiveSessionID) {
			controller.snapshotCache = controllerSnapshotCache{}
		}
		controller.mu.Unlock()
		_ = controller.snapshot()
	}
}

func benchmarkController() *controller {
	sessions := make([]desktopstate.SessionState, 64)
	for index := range sessions {
		sessions[index] = desktopstate.SessionState{
			ID:       "session-" + string(rune('a'+index)),
			Timeline: make([]desktopstate.TimelineItem, 128),
			Subagents: []desktopstate.SubagentState{{
				ID:      "agent",
				Profile: "strength",
				Status:  "running",
			}},
			Context: desktopstate.SessionContextState{
				Todo:   desktopstate.TodoState{Items: make([]desktopstate.TodoItemState, 16)},
				Memory: desktopstate.MemoryState{Workspace: make([]desktopstate.MemoryEntryState, 16), Global: make([]desktopstate.MemoryEntryState, 16)},
			},
		}
	}
	return &controller{
		state:         desktopstate.State{ActiveSessionID: sessions[0].ID, Sessions: sessions},
		profiles:      nil,
		connections:   nil,
		statuses:      nil,
		activeAgentID: controllerAgentID,
		histories:     make(map[string]historyState),
		revision:      1,
	}
}
