package desktop

import "testing"

func BenchmarkReduceSessionUpdate(b *testing.B) {
	state := benchmarkState()
	event := benchmarkSessionUpdate()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		state = Reduce(state, event)
	}
}

func BenchmarkApplySessionUpdate(b *testing.B) {
	state := benchmarkState()
	event := benchmarkSessionUpdate()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		Apply(&state, event)
	}
}

func BenchmarkCloneState(b *testing.B) {
	state := benchmarkState()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = CloneState(state)
	}
}

func benchmarkState() State {
	sessions := make([]SessionState, 64)
	for index := range sessions {
		sessions[index] = SessionState{
			ID:       sessionBenchmarkID(index),
			Timeline: make([]TimelineItem, 128),
			Subagents: []SubagentState{{
				ID:      "agent",
				Profile: "strength",
				Status:  "running",
			}},
			Context: SessionContextState{
				Todo:   TodoState{Items: make([]TodoItemState, 16)},
				Memory: MemoryState{Workspace: make([]MemoryEntryState, 16), Global: make([]MemoryEntryState, 16)},
			},
		}
	}
	return State{
		Sessions:        sessions,
		Projects:        make([]ProjectState, 16),
		PermissionInbox: make([]PermissionRequest, 16),
		Integrations:    make([]MCPIntegrationState, 16),
	}
}

func benchmarkSessionUpdate() Event {
	return Event{
		Kind:      EventTimelineUpserted,
		SessionID: sessionBenchmarkID(0),
		Item: TimelineItem{
			Kind: TimelineTool,
			ID:   "tool-0",
			Text: "updated",
		},
	}
}

func sessionBenchmarkID(index int) string {
	return "session-" + string(rune('a'+index))
}
