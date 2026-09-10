package agent

import "testing"

func TestMemoryResultStoreReturnsDetachedResults(t *testing.T) {
	store := newMemoryResultStore()
	ref := ResultRef{SessionID: "session-a", AgentID: "agility-1", Version: 3}
	original := Result{
		SessionID: "session-a",
		AgentID:   "agility-1",
		Profile:   ProfileAgility,
		Summary:   "finding",
		Evidence:  []EvidenceRef{{Tool: "read", Target: "agent.go"}},
	}

	store.Put(ref, original)
	got, ok := store.Get(ref)
	if !ok {
		t.Fatal("stored result missing")
	}
	got.Summary = "mutated"
	got.Evidence[0].Target = "changed.go"

	again, ok := store.Get(ref)
	if !ok {
		t.Fatal("stored result missing on second read")
	}
	if again.Summary != "finding" {
		t.Fatalf("summary = %q, want detached stored value", again.Summary)
	}
	if got := again.Evidence[0].Target; got != "agent.go" {
		t.Fatalf("evidence target = %q, want detached stored value", got)
	}
}

func TestMemoryResultStoreRejectsInvalidReference(t *testing.T) {
	store := newMemoryResultStore()
	store.Put(ResultRef{AgentID: "agility-1"}, Result{Summary: "ignored"})
	if _, ok := store.Get(ResultRef{AgentID: "agility-1"}); ok {
		t.Fatal("invalid zero-version reference was stored")
	}
}
