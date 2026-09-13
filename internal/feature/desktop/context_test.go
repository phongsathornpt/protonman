package desktop

import "testing"

func TestReduceSessionContextUpdated(t *testing.T) {
	state := State{Sessions: []SessionState{{ID: "s1"}}}
	ctx := SessionContextState{
		Goal: "finish desktop",
		Todo: TodoState{
			Revision: 3,
			Items:    []TodoItemState{{ID: "a", Text: "wire inspector", Status: "in_progress"}},
		},
	}

	next := Reduce(state, Event{Kind: EventSessionContextUpdated, SessionID: "s1", Context: ctx})
	if next.Sessions[0].Context.Goal != "finish desktop" {
		t.Fatalf("goal = %q", next.Sessions[0].Context.Goal)
	}
	if next.Sessions[0].Context.Todo.Revision != 3 || len(next.Sessions[0].Context.Todo.Items) != 1 {
		t.Fatalf("todo = %#v", next.Sessions[0].Context.Todo)
	}

	ctx.Todo.Items[0].Text = "mutated"
	if next.Sessions[0].Context.Todo.Items[0].Text != "wire inspector" {
		t.Fatalf("context aliased event payload: %#v", next.Sessions[0].Context.Todo.Items[0])
	}
}
