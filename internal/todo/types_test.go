package todo

import "testing"

func TestParseMarkdownStatusesAndStableIDs(t *testing.T) {
	input := "- [ ] one\n- [~] two\n- [x] three\n- [ ] one\n"
	got := ParseMarkdown(input)
	if len(got) != 4 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Status != StatusPending || got[1].Status != StatusInProgress || got[2].Status != StatusCompleted {
		t.Fatalf("statuses = %#v", got)
	}
	if got[0].ID == got[3].ID {
		t.Fatalf("duplicate text received duplicate id %q", got[0].ID)
	}
	again := ParseMarkdown(input)
	for i := range got {
		if got[i].ID != again[i].ID {
			t.Fatalf("id %d unstable: %q != %q", i, got[i].ID, again[i].ID)
		}
	}
}

func TestValidateItems(t *testing.T) {
	valid := []Item{{ID: "a", Text: "one", Status: StatusPending}, {ID: "b", Text: "one", Status: StatusCompleted}}
	if err := ValidateItems(valid); err != nil {
		t.Fatal(err)
	}
	dup := append(CloneItems(valid), Item{ID: "a", Text: "other", Status: StatusPending})
	if err := ValidateItems(dup); err == nil {
		t.Fatal("expected duplicate id error")
	}
}
