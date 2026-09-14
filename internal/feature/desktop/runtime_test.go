package desktop

import "testing"

func TestReduceUpdatesSessionRuntime(t *testing.T) {
	state := State{Sessions: []SessionState{{ID: "s1"}}}
	next := Reduce(state, Event{Kind: EventSessionRuntimeUpdated, SessionID: "s1", Runtime: RuntimeSettingsState{Provider: "protonman", Model: "qwen3.8-27b", Reasoning: "high", LowConcurrency: "on"}})
	got := next.Sessions[0].Runtime
	if got.Provider != "protonman" || got.Model != "qwen3.8-27b" || got.Reasoning != "high" || got.LowConcurrency != "on" {
		t.Fatalf("runtime = %#v", got)
	}
}

func TestRuntimeSurvivesContextUpdate(t *testing.T) {
	original := State{Sessions: []SessionState{{ID: "s1", Runtime: RuntimeSettingsState{Provider: "protonman", Model: "qwen"}}}}
	next := Reduce(original, Event{Kind: EventSessionContextUpdated, SessionID: "s1", Context: SessionContextState{Goal: "ship"}})
	if next.Sessions[0].Runtime != original.Sessions[0].Runtime {
		t.Fatalf("runtime lost: %#v", next.Sessions[0].Runtime)
	}
}
