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
}

func TestReduceDoesNotAliasTimeline(t *testing.T) {
	original := State{Sessions: []SessionState{{
		ID: "s1",
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
