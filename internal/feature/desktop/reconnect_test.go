package desktop

import "testing"

func TestMarkDisconnectedPreservesVisibleStateAndPausesInflightWork(t *testing.T) {
	state := State{
		ActiveSessionID: "running",
		Sessions: []SessionState{
			{ID: "running", Status: TaskRunning, Timeline: []TimelineItem{{Kind: TimelineAssistant, Text: "partial"}}, Subagents: []SubagentState{{ID: "a1", Profile: "strength", Status: "running"}}},
			{ID: "done", Status: TaskCompleted},
		},
		PermissionInbox: []PermissionRequest{{RequestID: "rpc-1", SessionID: "running"}},
	}

	next := MarkDisconnected(state)

	assertStatus(t, next, "running", TaskPaused)
	assertStatus(t, next, "done", TaskCompleted)
	if next.ActiveSessionID != "running" {
		t.Fatalf("active session = %q, want running", next.ActiveSessionID)
	}
	if len(next.Sessions[0].Timeline) != 1 || next.Sessions[0].Timeline[0].Text != "partial" {
		t.Fatalf("timeline not preserved: %#v", next.Sessions[0].Timeline)
	}
	if len(next.Sessions[0].Subagents) != 1 || next.Sessions[0].Subagents[0].ID != "a1" {
		t.Fatalf("subagents not preserved: %#v", next.Sessions[0].Subagents)
	}
	if len(next.PermissionInbox) != 0 {
		t.Fatalf("stale permission inbox preserved: %#v", next.PermissionInbox)
	}
}

func TestMarkDisconnectedDoesNotAliasOriginal(t *testing.T) {
	original := State{Sessions: []SessionState{{ID: "s1", Status: TaskRunning}}}
	next := MarkDisconnected(original)
	if original.Sessions[0].Status != TaskRunning {
		t.Fatalf("original state mutated: %#v", original.Sessions[0])
	}
	if next.Sessions[0].Status != TaskPaused {
		t.Fatalf("next state = %#v", next.Sessions[0])
	}
}
