package runtime

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
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

func TestAgentLifecycleWithTaskIDSchedulesTodoReload(t *testing.T) {
	repo := &reloadableTodoRepository{
		fixedTodoRepository: fixedTodoRepository{snapshot: tododomain.Snapshot{Revision: 1, Items: []tododomain.Item{{ID: "task-1", Text: "work", Status: tododomain.StatusPending}}}},
		reloaded:            tododomain.Snapshot{Revision: 2, Items: []tododomain.Item{{ID: "task-1", Text: "work", Status: tododomain.StatusInProgress}}},
	}
	m := newTestBubbleModel(t, permission.ModeAsk, repo.snapshot.Items)
	m.todoStore = repo
	m.todoRevision = repo.snapshot.Revision
	cmd := m.updateAgentLifecycle(agentLifecycleMsg{event: agent.Event{Kind: agent.EventAgentStarted, AgentID: "worker-1", TaskID: "task-1"}})
	if cmd == nil {
		t.Fatal("expected batch or reload command")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, subCmd := range batch {
			if subCmd != nil {
				if rmsg := subCmd(); rmsg != nil {
					if _, handled := m.updateRuntimeEvent(rmsg); handled {
						break
					}
				}
			}
		}
	} else if _, handled := m.updateRuntimeEvent(msg); !handled {
		t.Fatal("reload command was not handled")
	}
	if m.todoRevision != 2 || m.todo[0].Status != tododomain.StatusInProgress {
		t.Fatalf("todo status after lifecycle reload = %v, want in_progress", m.todo[0].Status)
	}
}
