package runtime

import (
	"context"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	"strings"
	"testing"
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

func TestTodoRetirementClearsWhenPlanReopens(t *testing.T) {
	repo := &fixedTodoRepository{snapshot: tododomain.Snapshot{Revision: 1, Items: []tododomain.Item{{ID: "a", Text: "ship", Status: tododomain.StatusCompleted}}}}
	m := newTestBubbleModel(t, permission.ModeAsk, repo.snapshot.Items)
	m.todoStore = repo
	m.todoRevision = 1
	m.retireCompletedTodoForNextTurn()
	if got := m.todoView(); got != "" {
		t.Fatalf("retired view=%q", got)
	}
	repo.snapshot = tododomain.Snapshot{Revision: 2, Items: []tododomain.Item{{ID: "a", Text: "ship follow-up", Status: tododomain.StatusPending}}}
	if !m.syncTodoSnapshot() {
		t.Fatal("reopened plan was not synchronized")
	}
	if m.todoLifecycle.CompletionDismissed || !strings.Contains(m.todoView(), "Tasks 0/1") {
		t.Fatalf("reopened lifecycle=%+v view=%q", m.todoLifecycle, m.todoView())
	}
}
