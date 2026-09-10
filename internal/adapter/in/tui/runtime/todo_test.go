package runtime

import (
	"context"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
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
	m := newTestBubbleModel(t, permission.ModeAsk, []tododomain.Item{{ID: "a", Text: "old text", Status: tododomain.StatusPending}})
	m.todoStore = repo
	m.todoRevision = 4
	if !m.syncTodoSnapshot() {
		t.Fatal("same-revision content change was ignored")
	}
	if got := m.todo[0].Text; got != "new text" {
		t.Fatalf("todo text=%q, want new text", got)
	}
}

type reloadableTodoRepository struct {
	fixedTodoRepository
	reloaded tododomain.Snapshot
}

func (r *reloadableTodoRepository) Reload(context.Context) (tododomain.Snapshot, error) {
	r.snapshot = tododomain.CloneSnapshot(r.reloaded)
	return tododomain.CloneSnapshot(r.reloaded), nil
}

func TestOpeningTodoPaneReloadsDurableSnapshot(t *testing.T) {
	repo := &reloadableTodoRepository{
		fixedTodoRepository: fixedTodoRepository{snapshot: tododomain.Snapshot{Revision: 1, Items: []tododomain.Item{{ID: "a", Text: "stale", Status: tododomain.StatusPending}}}},
		reloaded:            tododomain.Snapshot{Revision: 2, Items: []tododomain.Item{{ID: "a", Text: "fresh", Status: tododomain.StatusInProgress}}},
	}
	m := newTestBubbleModel(t, permission.ModeAsk, repo.snapshot.Items)
	m.todoStore = repo
	m.todoRevision = repo.snapshot.Revision
	cmd := m.openTodoPane()
	if cmd == nil {
		t.Fatal("opening todo pane did not schedule durable reload")
	}
	msg, ok := cmd().(todoReloadedMsg)
	if !ok {
		t.Fatalf("reload command message=%T", cmd())
	}
	if _, handled := m.updateRuntimeEvent(msg); !handled {
		t.Fatal("todo reload message was not handled")
	}
	if m.todoRevision != 2 || len(m.todo) != 1 || m.todo[0].Text != "fresh" || m.todo[0].Status != tododomain.StatusInProgress {
		t.Fatalf("todo revision=%d items=%+v", m.todoRevision, m.todo)
	}
}
