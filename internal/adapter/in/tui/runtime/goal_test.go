package runtime

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	turnmsg "github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/turn"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type goalTestModel struct{}

func (*goalTestModel) Provider() string { return "test" }
func (*goalTestModel) ModelID() string  { return "goal-test" }
func (*goalTestModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true}
}
func (*goalTestModel) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	return &goalTestStream{}, nil
}

type goalTestStream struct{ index int }

func (s *goalTestStream) Next(context.Context) (sdk.Event, error) {
	switch s.index {
	case 0:
		s.index++
		return sdk.Event{Kind: sdk.EventTextDelta, Text: "done"}, nil
	case 1:
		s.index++
		return sdk.Event{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}, nil
	default:
		return sdk.Event{}, io.EOF
	}
}
func (*goalTestStream) Close() error { return nil }

func TestGoalCommandStartsExecutionTurn(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	runner, err := turn.NewLoop(&goalTestModel{}, service, turn.WithSystemPromptSpec(prompt.Spec{}))
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(context.Background(), service, registry, nil, runner, newPermissionBridge(), "")
	cmd := m.executeCommand("/goal implement retry recovery")
	if cmd == nil {
		t.Fatal("/goal with detail did not start a turn")
	}
	if !m.busy {
		t.Fatal("/goal with detail did not mark the turn busy")
	}
	if len(m.conversation.Messages()) != 1 || m.conversation.Messages()[0].Content != "implement retry recovery" {
		t.Fatalf("goal execution messages = %#v", m.conversation.Messages())
	}
}

func TestGoalCommandDoesNotMutateGoalDuringActiveTurn(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	runner, err := turn.NewLoop(&goalTestModel{}, service, turn.WithSystemPromptSpec(prompt.Spec{}))
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(context.Background(), service, registry, nil, runner, newPermissionBridge(), "")
	m.activeGoal = "existing goal"
	m.busy = true
	m.activeTurnOwner = "turn-existing"
	if cmd := m.executeCommand("/goal replacement goal"); cmd != nil {
		t.Fatalf("busy /goal returned command: %v", cmd)
	}
	if m.activeGoal != "existing goal" || m.activeTurnOwner != "turn-existing" {
		t.Fatalf("busy /goal mutated state: goal=%q owner=%q", m.activeGoal, m.activeTurnOwner)
	}
}

func TestStartTurnPreservesActiveTurnOwnership(t *testing.T) {
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	m := newBubbleModel(context.Background(), service, registry, nil, nil, newPermissionBridge(), "")
	m.busy = true
	m.activeTurnOwner = "turn-existing"
	originalEvents := make(chan tea.Msg)
	m.turnEvents = originalEvents
	if cmd := m.startTurn("second turn"); cmd != nil {
		t.Fatalf("busy startTurn returned command: %v", cmd)
	}
	if m.activeTurnOwner != "turn-existing" || m.turnEvents != originalEvents {
		t.Fatalf("startTurn overwrote active ownership: owner=%q events_same=%v", m.activeTurnOwner, m.turnEvents == originalEvents)
	}
}

func TestSetActiveGoalSupersedesBoundTodoPlan(t *testing.T) {
	ctx := context.Background()
	store, err := tododomain.OpenMarkdownStore(ctx, filepath.Join(t.TempDir(), "todo.md"))
	if err != nil {
		t.Fatal(err)
	}
	bound, _, err := store.BindGoal(ctx, "old goal")
	if err != nil {
		t.Fatal(err)
	}
	withTask, err := store.CompareAndReplace(ctx, bound.Revision, []tododomain.Item{{ID: "old", Text: "old task", Status: tododomain.StatusInProgress}})
	if err != nil {
		t.Fatal(err)
	}
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	runner, err := turn.NewLoop(&goalTestModel{}, service, turn.WithSystemPromptSpec(prompt.Spec{ActiveGoal: "old goal"}))
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, service, registry, withTask.Items, runner, newPermissionBridge(), "")
	m.todoStore = store
	m.todoRevision = withTask.Revision
	m.activeGoal = "old goal"
	if err := m.setActiveGoal("new goal"); err != nil {
		t.Fatal(err)
	}
	if m.activeGoal != "new goal" {
		t.Fatalf("active goal=%q", m.activeGoal)
	}
	if len(m.todo) != 0 || len(store.Snapshot().Items) != 0 {
		t.Fatalf("stale todo survived goal change: model=%+v store=%+v", m.todo, store.Snapshot())
	}
}

func TestCompletedGoalResultClearsPersistentGoal(t *testing.T) {
	ctx := context.Background()
	store, err := tododomain.OpenMarkdownStore(ctx, filepath.Join(t.TempDir(), "todo.md"))
	if err != nil {
		t.Fatal(err)
	}
	bound, _, err := store.BindGoal(ctx, "finish work")
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.CompareAndReplace(ctx, bound.Revision, []tododomain.Item{{ID: "done", Text: "finish work", Status: tododomain.StatusCompleted}})
	if err != nil {
		t.Fatal(err)
	}
	registry := behaviorRegistry{handler: &countingHandler{definition: tool.Definition{Name: "read", Kind: tool.KindRead}}}
	service := newBehaviorService(t, registry, permission.ModeAsk)
	runner, err := turn.NewLoop(&goalTestModel{}, service, turn.WithSystemPromptSpec(prompt.Spec{ActiveGoal: "finish work"}))
	if err != nil {
		t.Fatal(err)
	}
	m := newBubbleModel(ctx, service, registry, completed.Items, runner, newPermissionBridge(), "")
	m.todoStore = store
	m.todoRevision = completed.Revision
	m.todoLifecycle.CompletionFresh = true
	m.activeGoal = "finish work"
	m.busy = true
	m.updateTurnDone(turnmsg.Done{Result: turn.Result{GoalCompleted: true}})
	if m.activeGoal != "" {
		t.Fatalf("active goal after completed result = %q", m.activeGoal)
	}
	if m.todoLifecycle.CompletionFresh || !m.todoLifecycle.CompletionDismissed {
		t.Fatalf("todo completion lifecycle = %+v", m.todoLifecycle)
	}
}
