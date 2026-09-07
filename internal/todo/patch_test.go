package todo

import "testing"

func TestApplyPatchPreservesUnmentionedItems(t *testing.T) {
	before := []Item{
		{ID: "a", Text: "inspect", Status: StatusPending},
		{ID: "b", Text: "fix", Status: StatusInProgress},
		{ID: "c", Text: "verify", Status: StatusPending},
	}
	after, err := ApplyPatch(before, []Operation{{Op: PatchSetStatus, ID: "b", Status: StatusCompleted}})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 3 || after[0] != before[0] || after[2] != before[2] || after[1].Status != StatusCompleted {
		t.Fatalf("after = %#v", after)
	}
	if before[1].Status != StatusInProgress {
		t.Fatal("ApplyPatch mutated input slice")
	}
}

func TestApplyPatchSupportsExplicitAddEditAndRemove(t *testing.T) {
	before := []Item{{ID: "a", Text: "old", Status: StatusPending}, {ID: "remove", Text: "remove", Status: StatusPending}}
	after, err := ApplyPatch(before, []Operation{
		{Op: PatchSetText, ID: "a", Text: "new"},
		{Op: PatchSetStatus, ID: "a", Status: StatusInProgress},
		{Op: PatchRemove, ID: "remove"},
		{Op: PatchAdd, ID: "b", Text: "verify", Status: StatusPending},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 || after[0].ID != "a" || after[0].Text != "new" || after[0].Status != StatusInProgress || after[1].ID != "b" {
		t.Fatalf("after = %#v", after)
	}
}

func TestApplyPatchRejectsImplicitOrAmbiguousMutations(t *testing.T) {
	before := []Item{{ID: "a", Text: "one", Status: StatusPending}}
	tests := [][]Operation{
		nil,
		{{Op: PatchRemove, ID: "missing"}},
		{{Op: PatchAdd, ID: "a", Text: "duplicate", Status: StatusPending}},
		{{Op: PatchSetStatus, ID: "a", Status: StatusCompleted, Text: "also rename"}},
		{{Op: PatchSetText, ID: "a", Text: "rename", Status: StatusCompleted}},
		{{Op: PatchOp("replace_all"), ID: "a"}},
	}
	for _, operations := range tests {
		if _, err := ApplyPatch(before, operations); err == nil {
			t.Fatalf("ApplyPatch(%#v) error = nil", operations)
		}
	}
}
