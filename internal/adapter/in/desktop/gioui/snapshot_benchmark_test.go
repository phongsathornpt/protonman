//go:build desktop || desktop_gio

package gioui

import (
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
