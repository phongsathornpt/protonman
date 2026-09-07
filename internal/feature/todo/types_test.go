package todo

import (
	"encoding/json"
	"testing"
)

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

func TestValidateItemsRejectsUnsafeMarkdownFields(t *testing.T) {
	tests := []Item{
		{ID: "bad]id", Text: "safe", Status: StatusPending},
		{ID: "safe", Text: "line one\nline two", Status: StatusPending},
		{ID: "safe", Text: managedEnd, Status: StatusPending},
	}
	for _, item := range tests {
		if err := ValidateItems([]Item{item}); err == nil {
			t.Fatalf("ValidateItems(%#v) error=nil", item)
		}
	}
}

func TestCloneItemsPreservesEmptyArrayJSONShape(t *testing.T) {
	items := CloneItems(nil)
	if items == nil {
		t.Fatal("CloneItems(nil) = nil, want non-nil empty slice")
	}
	payload, err := json.Marshal(Snapshot{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(payload); got != `{"revision":0,"items":[]}` {
		t.Fatalf("snapshot JSON = %s, want empty items array", got)
	}
}

func TestActiveItemsAllowsConcurrentInProgressTasks(t *testing.T) {
	items := []Item{
		{ID: "root", Text: "root work", Status: StatusInProgress},
		{ID: "child", Text: "delegated work", Status: StatusInProgress},
		{ID: "later", Text: "later", Status: StatusPending},
	}
	if err := ValidateItems(items); err != nil {
		t.Fatalf("concurrent active tasks should be valid: %v", err)
	}
	active := ActiveItems(items)
	if len(active) != 2 || active[0].ID != "root" || active[1].ID != "child" {
		t.Fatalf("active=%#v", active)
	}
}
