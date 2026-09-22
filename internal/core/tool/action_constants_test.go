package tool

import "testing"

// Facade action strings are a wire contract: JSON schemas, permission policy,
// turn safety budgets, and lifecycle presentation all match on exact values.
func TestSubagentActionValuesAreStable(t *testing.T) {
	want := map[string]string{
		ActionSpawn:  "spawn",
		ActionWait:   "wait",
		ActionGet:    "get",
		ActionList:   "list",
		ActionCancel: "cancel",
		ActionResume: "resume",
	}
	for got, expected := range want {
		if got != expected {
			t.Fatalf("action constant = %q, want %q", got, expected)
		}
	}
}
