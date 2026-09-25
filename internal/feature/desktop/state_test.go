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

func TestCloneStateDoesNotAliasNestedData(t *testing.T) {
	original := State{
		Projects: []ProjectState{{ID: "project", Folders: []ProjectFolder{{Path: "/workspace"}}}},
		Sessions: []SessionState{{
			ID:        "session",
			Timeline:  []TimelineItem{{ID: "item"}},
			Subagents: []SubagentState{{ID: "agent"}},
			Context: SessionContextState{
				Todo:   TodoState{Items: []TodoItemState{{ID: "todo"}}},
				Memory: MemoryState{Workspace: []MemoryEntryState{{ID: "memory"}}},
			},
		}},
		PermissionInbox: []PermissionRequest{{RequestID: "permission", Options: []PermissionOption{{ID: "allow"}}}},
		Integrations:    []MCPIntegrationState{{Name: "mcp", Args: []string{"arg"}}},
	}

	clone := CloneState(original)
	clone.Projects[0].Folders[0].Path = "/changed"
	clone.Sessions[0].Timeline[0].ID = "changed"
	clone.Sessions[0].Subagents[0].ID = "changed"
	clone.Sessions[0].Context.Todo.Items[0].ID = "changed"
	clone.Sessions[0].Context.Memory.Workspace[0].ID = "changed"
	clone.PermissionInbox[0].Options[0].ID = "changed"
	clone.Integrations[0].Args[0] = "changed"

	if original.Projects[0].Folders[0].Path != "/workspace" ||
		original.Sessions[0].Timeline[0].ID != "item" ||
		original.Sessions[0].Subagents[0].ID != "agent" ||
		original.Sessions[0].Context.Todo.Items[0].ID != "todo" ||
		original.Sessions[0].Context.Memory.Workspace[0].ID != "memory" ||
		original.PermissionInbox[0].Options[0].ID != "allow" ||
		original.Integrations[0].Args[0] != "arg" {
		t.Fatal("CloneState aliased nested state")
	}
}

func TestApplyCopiesEventOwnedSlices(t *testing.T) {
	state := State{Sessions: []SessionState{{ID: "s1"}}}
	todo := TodoState{Items: []TodoItemState{{ID: "todo", Text: "ship"}}}
	memory := MemoryState{Workspace: []MemoryEntryState{{ID: "memory", Value: "keep"}}}
	integrations := []MCPIntegrationState{{Name: "docs", Args: []string{"--stdio"}}}
	permission := PermissionRequest{
		RequestID: "permission",
		Options:   []PermissionOption{{ID: "allow", Name: "Allow"}},
	}

	Apply(&state, Event{Kind: EventSessionContextUpdated, SessionID: "s1", Context: SessionContextState{Todo: todo}})
	Apply(&state, Event{Kind: EventSessionMemoryUpdated, SessionID: "s1", Memory: memory})
	Apply(&state, Event{Kind: EventIntegrationsReplaced, Integrations: integrations})
	Apply(&state, Event{Kind: EventPermissionRequested, SessionID: "s1", Permission: permission})

	todo.Items[0].Text = "changed"
	memory.Workspace[0].Value = "changed"
	integrations[0].Args[0] = "changed"
	permission.Options[0].Name = "changed"

	session := state.Sessions[0]
	if session.Context.Todo.Items[0].Text != "ship" ||
		session.Context.Memory.Workspace[0].Value != "keep" ||
		state.Integrations[0].Args[0] != "--stdio" ||
		state.PermissionInbox[0].Options[0].Name != "Allow" {
		t.Fatalf("Apply retained event-owned slices: %#v", state)
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
