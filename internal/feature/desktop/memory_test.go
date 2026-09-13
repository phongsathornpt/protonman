package desktop

import "testing"

func TestSessionMemoryUpdatedPreservesGoalTodoAndOwnsSlices(t *testing.T) {
	state := State{Sessions: []SessionState{{
		ID: "s1",
		Context: SessionContextState{
			Goal: "ship desktop",
			Todo: TodoState{Revision: 3, Items: []TodoItemState{{ID: "todo-1", Text: "wire memory", Status: "in_progress"}}},
		},
	}}}
	memory := MemoryState{
		WorkspaceKey: "ws-1",
		Workspace: []MemoryEntryState{{ID: "m1", Scope: "workspace", Kind: "repo_fact", Key: "path", Value: "repo root", Confidence: 0.9}},
		Global: []MemoryEntryState{{ID: "m2", Scope: "global", Kind: "preference", Key: "style", Value: "concise", Confidence: 0.95}},
	}

	next := Reduce(state, Event{Kind: EventSessionMemoryUpdated, SessionID: "s1", Memory: memory})
	memory.Workspace[0].Value = "mutated"
	memory.Global[0].Value = "mutated"

	session := next.Sessions[0]
	if session.Context.Goal != "ship desktop" || session.Context.Todo.Revision != 3 {
		t.Fatalf("goal/todo changed: %#v", session.Context)
	}
	if got := session.Context.Memory.Workspace[0].Value; got != "repo root" {
		t.Fatalf("workspace memory aliased caller slice: %q", got)
	}
	if got := session.Context.Memory.Global[0].Value; got != "concise" {
		t.Fatalf("global memory aliased caller slice: %q", got)
	}
}

func TestSessionContextUpdatedPreservesMemory(t *testing.T) {
	state := State{Sessions: []SessionState{{
		ID: "s1",
		Context: SessionContextState{Memory: MemoryState{Workspace: []MemoryEntryState{{ID: "m1", Value: "keep"}}}},
	}}}

	next := Reduce(state, Event{
		Kind:      EventSessionContextUpdated,
		SessionID: "s1",
		Context: SessionContextState{Goal: "new goal", Todo: TodoState{Revision: 4}},
	})

	if next.Sessions[0].Context.Goal != "new goal" {
		t.Fatalf("goal not updated: %#v", next.Sessions[0].Context)
	}
	if got := next.Sessions[0].Context.Memory.Workspace[0].Value; got != "keep" {
		t.Fatalf("memory lost during context update: %q", got)
	}
}
