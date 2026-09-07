package tui

import (
	"context"
	"testing"

	"github.com/projectTHORN/proton/internal/core/permission"
	tododomain "github.com/projectTHORN/proton/internal/feature/todo"
)

type fixedTodoRepository struct{ snapshot tododomain.Snapshot }

func (r *fixedTodoRepository) Snapshot() tododomain.Snapshot {
	return tododomain.CloneSnapshot(r.snapshot)
}
func (r *fixedTodoRepository) CompareAndReplace(context.Context, uint64, []tododomain.Item) (tododomain.Snapshot, error) {
	return tododomain.CloneSnapshot(r.snapshot), nil
}

func TestSyncTodoSnapshotDetectsContentChangeAtStableRevision(t *testing.T) {
	repo := &fixedTodoRepository{snapshot: tododomain.Snapshot{Revision: 4, Items: []tododomain.Item{{ID: "a", Text: "new text", Status: tododomain.StatusPending}}}}
	m := newTestBubbleModel(t, permission.ModeAsk, []TodoItem{{ID: "a", Text: "old text", Status: tododomain.StatusPending}})
	m.todoStore = repo
	m.todoRevision = 4
	if !m.syncTodoSnapshot() {
		t.Fatal("same-revision content change was ignored")
	}
	if got := m.todo[0].Text; got != "new text" {
		t.Fatalf("todo text=%q, want new text", got)
	}
}
