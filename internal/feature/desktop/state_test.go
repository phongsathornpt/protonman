package desktop

import "testing"

func TestReduceSessionLifecycle(t *testing.T) {
	state := State{Sessions: []SessionState{{ID: "s1", Status: TaskIdle}}}

	state = Reduce(state, Event{Kind: EventPromptQueued, SessionID: "s1"})
	assertStatus(t, state, "s1", TaskQueued)

	state = Reduce(state, Event{Kind: EventPromptStarted, SessionID: "s1"})
	assertStatus(t, state, "s1", TaskRunning)

	state = Reduce(state, Event{Kind: EventPermissionRequested, SessionID: "s1"})
	assertStatus(t, state, "s1", TaskWaitingPermission)

	state = Reduce(state, Event{Kind: EventPermissionResolved, SessionID: "s1"})
	assertStatus(t, state, "s1", TaskRunning)

	state = Reduce(state, Event{Kind: EventPromptCompleted, SessionID: "s1"})
	assertStatus(t, state, "s1", TaskCompleted)
}

func TestReducePreservesIndependentSessionLifecycle(t *testing.T) {
	state := State{Sessions: []SessionState{
		{ID: "a", Status: TaskIdle},
		{ID: "b", Status: TaskIdle},
	}}

	state = Reduce(state, Event{Kind: EventPromptStarted, SessionID: "a"})
	state = Reduce(state, Event{Kind: EventPermissionRequested, SessionID: "b"})

	assertStatus(t, state, "a", TaskRunning)
	assertStatus(t, state, "b", TaskWaitingPermission)
}

func TestReduceSessionSelectionRequiresKnownSession(t *testing.T) {
	state := State{Sessions: []SessionState{{ID: "known"}}}

	state = Reduce(state, Event{Kind: EventSessionSelected, SessionID: "missing"})
	if state.ActiveSessionID != "" {
		t.Fatalf("unknown session selected: %q", state.ActiveSessionID)
	}

	state = Reduce(state, Event{Kind: EventSessionSelected, SessionID: "known"})
	if state.ActiveSessionID != "known" {
		t.Fatalf("active session = %q, want known", state.ActiveSessionID)
	}

	state = Reduce(state, Event{Kind: EventSessionDeselected})
	if state.ActiveSessionID != "" {
		t.Fatalf("active session after deselect = %q, want empty", state.ActiveSessionID)
	}
}

func TestReduceDoesNotAliasTimeline(t *testing.T) {
	original := State{Sessions: []SessionState{{
		ID:       "s1",
		Timeline: []TimelineItem{{Kind: TimelineStatus, Text: "original"}},
	}}}

	next := Reduce(original, Event{
		Kind:      EventTimelineAppended,
		SessionID: "s1",
		Item:      TimelineItem{Kind: TimelineAssistant, Text: "new"},
	})

	if len(original.Sessions[0].Timeline) != 1 {
		t.Fatalf("original timeline mutated: %#v", original.Sessions[0].Timeline)
	}
	if len(next.Sessions[0].Timeline) != 2 {
		t.Fatalf("next timeline length = %d, want 2", len(next.Sessions[0].Timeline))
	}
}

func TestReduceTracksPermissionInbox(t *testing.T) {
	state := State{Sessions: []SessionState{{ID: "s1", Status: TaskRunning}}}
	permission := PermissionRequest{
		RequestID: "rpc-7",
		SessionID: "s1",
		Title:     "Bash",
		Options:   []PermissionOption{{ID: "allow", Name: "Allow once", Kind: "allow_once"}},
	}

	state = Reduce(state, Event{Kind: EventPermissionRequested, SessionID: "s1", Permission: permission})
	assertStatus(t, state, "s1", TaskWaitingPermission)
	if len(state.PermissionInbox) != 1 || state.PermissionInbox[0].RequestID != "rpc-7" {
		t.Fatalf("permission inbox = %#v", state.PermissionInbox)
	}

	state = Reduce(state, Event{Kind: EventPermissionRequested, SessionID: "s1", Permission: permission})
	if len(state.PermissionInbox) != 1 {
		t.Fatalf("duplicate permission request added: %#v", state.PermissionInbox)
	}

	state = Reduce(state, Event{Kind: EventPermissionResolved, SessionID: "s1", RequestID: "rpc-7"})
	assertStatus(t, state, "s1", TaskRunning)
	if len(state.PermissionInbox) != 0 {
		t.Fatalf("permission inbox not cleared: %#v", state.PermissionInbox)
	}
}

func TestReduceResetsSessionTimelineAndPermissions(t *testing.T) {
	state := State{
		Sessions:        []SessionState{{ID: "s1", Timeline: []TimelineItem{{Kind: TimelineAssistant, Text: "old"}}, Subagents: []SubagentState{{ID: "a1"}}}},
		PermissionInbox: []PermissionRequest{{RequestID: "r1"}},
	}

	state = Reduce(state, Event{Kind: EventSessionTimelineReset, SessionID: "s1"})
	state = Reduce(state, Event{Kind: EventPermissionsCleared})

	if len(state.Sessions[0].Timeline) != 0 || len(state.Sessions[0].Subagents) != 0 {
		t.Fatalf("session projection was not reset: %#v", state.Sessions[0])
	}
	if len(state.PermissionInbox) != 0 {
		t.Fatalf("permission inbox was not cleared: %#v", state.PermissionInbox)
	}
}

func TestReduceDoesNotAliasPermissionOptions(t *testing.T) {
	original := State{PermissionInbox: []PermissionRequest{{
		RequestID: "r1",
		Options:   []PermissionOption{{ID: "o1", Name: "Allow once"}},
	}}}

	next := Reduce(original, Event{})
	next.PermissionInbox[0].Options[0].Name = "changed"
	if original.PermissionInbox[0].Options[0].Name != "Allow once" {
		t.Fatal("permission options aliased between states")
	}
}

func TestReduceTracksAgentHealth(t *testing.T) {
	state := State{}
	state = Reduce(state, Event{
		Kind: EventAgentHealthUpdated,
		AgentHealth: AgentHealthState{
			ID:     "antigravity",
			Status: AgentStatusConnected,
		},
	})
	if health, ok := state.AgentHealth["antigravity"]; !ok || health.Status != AgentStatusConnected {
		t.Fatalf("unexpected agent health: %#v", state.AgentHealth)
	}

	state = MarkAgentDisconnected(state, "antigravity")
	if health := state.AgentHealth["antigravity"]; health.Status != AgentStatusDisconnected {
		t.Fatalf("expected disconnected status, got: %#v", health)
	}
}

func TestReduceTracksActiveAgent(t *testing.T) {
	state := State{ActiveAgentID: "protonman"}
	state = Reduce(state, Event{Kind: EventActiveAgentChanged, AgentID: "antigravity"})
	if state.ActiveAgentID != "antigravity" {
		t.Fatalf("active agent = %q, want antigravity", state.ActiveAgentID)
	}
}

func assertStatus(t *testing.T, state State, id string, want TaskStatus) {
	t.Helper()
	for _, session := range state.Sessions {
		if session.ID == id {
			if session.Status != want {
				t.Fatalf("session %s status = %q, want %q", id, session.Status, want)
			}
			return
		}
	}
	t.Fatalf("session %s not found", id)
}
