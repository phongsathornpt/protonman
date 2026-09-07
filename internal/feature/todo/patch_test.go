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

func TestClassifyPatchImpact(t *testing.T) {
	tests := []struct {
		name string
		ops  []Operation
		want PatchImpact
	}{
		{name: "empty", want: PatchImpactUnknown},
		{name: "status only", ops: []Operation{{Op: PatchSetStatus, ID: "a", Status: StatusCompleted}}, want: PatchImpactStatusOnly},
		{name: "multiple status", ops: []Operation{{Op: PatchSetStatus, ID: "a", Status: StatusCompleted}, {Op: PatchSetStatus, ID: "b", Status: StatusInProgress}}, want: PatchImpactStatusOnly},
		{name: "add", ops: []Operation{{Op: PatchAdd, ID: "a", Text: "a", Status: StatusPending}}, want: PatchImpactStructural},
		{name: "text", ops: []Operation{{Op: PatchSetText, ID: "a", Text: "rename"}}, want: PatchImpactStructural},
		{name: "remove", ops: []Operation{{Op: PatchRemove, ID: "a"}}, want: PatchImpactStructural},
		{name: "mixed", ops: []Operation{{Op: PatchSetStatus, ID: "a", Status: StatusCompleted}, {Op: PatchRemove, ID: "b"}}, want: PatchImpactStructural},
		{name: "unknown", ops: []Operation{{Op: PatchOp("dance"), ID: "a"}}, want: PatchImpactUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyPatch(tt.ops); got != tt.want {
				t.Fatalf("ClassifyPatch() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyPatchRejectsConflictingWritesWithinBatch(t *testing.T) {
	items := []Item{{ID: "a", Text: "old", Status: StatusPending}}
	cases := [][]Operation{
		{{Op: PatchSetStatus, ID: "a", Status: StatusInProgress}, {Op: PatchSetStatus, ID: "a", Status: StatusCompleted}},
		{{Op: PatchSetText, ID: "a", Text: "one"}, {Op: PatchSetText, ID: "a", Text: "two"}},
		{{Op: PatchRemove, ID: "a"}, {Op: PatchAdd, ID: "a", Text: "new", Status: StatusPending}},
	}
	for _, operations := range cases {
		if _, err := ApplyPatch(items, operations); err == nil {
			t.Fatalf("operations %#v unexpectedly accepted", operations)
		}
	}
}

func TestApplyPatchAllowsIndependentFieldWritesOnSameTask(t *testing.T) {
	items := []Item{{ID: "a", Text: "old", Status: StatusPending}}
	next, err := ApplyPatch(items, []Operation{
		{Op: PatchSetText, ID: "a", Text: "new"},
		{Op: PatchSetStatus, ID: "a", Status: StatusInProgress},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next[0].Text != "new" || next[0].Status != StatusInProgress {
		t.Fatalf("next=%#v", next)
	}
}

func TestEffectsOfPatchSeparatesTextStatusAndStructuralWrites(t *testing.T) {
	effects := EffectsOfPatch([]Operation{{Op: PatchSetText, ID: "a"}, {Op: PatchSetStatus, ID: "a"}})
	if !effects.Valid || !effects.Text || !effects.Status || effects.Structural {
		t.Fatalf("effects=%+v", effects)
	}
	if got := ClassifyPatch([]Operation{{Op: PatchSetText, ID: "a"}}); got != PatchImpactStructural {
		t.Fatalf("text patch impact=%q", got)
	}
}
