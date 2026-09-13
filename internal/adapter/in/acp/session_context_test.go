package acp

import (
	"testing"

	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
)

func TestProjectTodoSnapshot(t *testing.T) {
	snapshot := tododomain.Snapshot{
		Revision: 7,
		Items: []tododomain.Item{
			{ID: "a", Text: "inspect ACP", Status: tododomain.StatusInProgress},
			{ID: "b", Text: "render desktop", Status: tododomain.StatusPending},
		},
	}

	got := projectTodoSnapshot(snapshot)
	if got.Revision != 7 {
		t.Fatalf("revision = %d, want 7", got.Revision)
	}
	if len(got.Items) != 2 {
		t.Fatalf("items = %#v", got.Items)
	}
	if got.Items[0].ID != "a" || got.Items[0].Status != "in_progress" {
		t.Fatalf("first item = %#v", got.Items[0])
	}
	if got.Items[1].Text != "render desktop" || got.Items[1].Status != "pending" {
		t.Fatalf("second item = %#v", got.Items[1])
	}
}

func TestProjectTodoSnapshotDoesNotAliasInput(t *testing.T) {
	input := tododomain.Snapshot{
		Revision: 1,
		Items:    []tododomain.Item{{ID: "a", Text: "original", Status: tododomain.StatusPending}},
	}
	got := projectTodoSnapshot(input)
	input.Items[0].Text = "mutated"
	if got.Items[0].Text != "original" {
		t.Fatalf("projection aliased input: %#v", got.Items[0])
	}
}
