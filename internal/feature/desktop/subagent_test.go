package desktop

import "testing"

func TestReduceUpsertsSubagentParticipant(t *testing.T) {
	state := State{Sessions: []SessionState{{ID: "s1"}}}
	state = Reduce(state, Event{
		Kind:      EventSubagentUpserted,
		SessionID: "s1",
		Subagent: SubagentState{
			ID:      "agent-1",
			Profile: "strength",
			Task:    "Refactor ACP client",
			Status:  "running",
		},
	})
	state = Reduce(state, Event{
		Kind:      EventSubagentUpserted,
		SessionID: "s1",
		Subagent: SubagentState{
			ID:      "agent-1",
			Summary: "Shared protocol extraction complete",
			Status:  "completed",
		},
	})

	got := state.Sessions[0].Subagents
	if len(got) != 1 {
		t.Fatalf("subagents len = %d, want 1", len(got))
	}
	if got[0].Profile != "strength" || got[0].Task != "Refactor ACP client" {
		t.Fatalf("participant fields were not preserved: %#v", got[0])
	}
	if got[0].Status != "completed" || got[0].Summary == "" {
		t.Fatalf("participant terminal update missing: %#v", got[0])
	}
}

func TestReduceDoesNotAliasSubagents(t *testing.T) {
	original := State{Sessions: []SessionState{{
		ID:        "s1",
		Subagents: []SubagentState{{ID: "a1", Profile: "agility"}},
	}}}
	next := Reduce(original, Event{})
	next.Sessions[0].Subagents[0].Profile = "changed"
	if original.Sessions[0].Subagents[0].Profile != "agility" {
		t.Fatal("subagent state aliased between reducer states")
	}
}
