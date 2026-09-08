package agent

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

func TestWaitActivityForTurnReturnsOrderedCompletionBatch(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil,
		WithMaxConcurrency(4),
		WithRunnerFactory(func(Profile, *toolcall.Service) (turn.Runner, error) {
			return hardeningRunner{}, nil
		}),
	)
	defer coord.Close()
	for i := 0; i < 3; i++ {
		if _, err := coord.Spawn(context.Background(), Request{
			SessionID: "session-a", ParentID: "turn-1", Profile: ProfileAgility,
			Task: fmt.Sprintf("task-%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(time.Second)
	for len(coord.Active()) != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	result, err := coord.WaitActivityForTurn(context.Background(), TurnRef{SessionID: "session-a", TurnID: "turn-1"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != 3 {
		t.Fatalf("events = %#v", result.Events)
	}
	if result.Cursor != 3 || result.Event == nil || result.Event.AgentID != result.Events[2].AgentID {
		t.Fatalf("result = %+v", result)
	}
	if result.Truncated {
		t.Fatal("unexpected truncated activity batch")
	}

	timedOut, err := coord.WaitActivityForTurn(context.Background(), TurnRef{SessionID: "session-a", TurnID: "turn-1"}, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !timedOut.TimedOut || len(timedOut.Events) != 0 {
		t.Fatalf("second wait = %+v", timedOut)
	}
}

func TestWaitActivityAfterSupportsIndependentCursors(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	ref := TurnRef{SessionID: "session-a", TurnID: "turn-2"}
	coord.recordActivity(Event{Kind: EventAgentCompleted, SessionID: ref.SessionID, ParentID: ref.TurnID, AgentID: "agility-1", Profile: ProfileAgility})
	coord.recordActivity(Event{Kind: EventAgentFailed, SessionID: ref.SessionID, ParentID: ref.TurnID, AgentID: "strength-2", Profile: ProfileStrength})
	first, err := coord.WaitActivityAfter(context.Background(), ref, 0, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second, err := coord.WaitActivityAfter(context.Background(), ref, 0, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 2 || len(second.Events) != 2 || first.Cursor != 2 || second.Cursor != 2 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
	afterFirst, err := coord.WaitActivityAfter(context.Background(), ref, 1, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterFirst.Events) != 1 || afterFirst.Events[0].AgentID != "strength-2" || afterFirst.Cursor != 2 {
		t.Fatalf("after cursor 1 = %+v", afterFirst)
	}
}

func TestActivityStreamReportsTruncationAfterRetentionBoundary(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	ref := TurnRef{SessionID: "session-a", TurnID: "turn-3"}
	for i := 0; i < maxActivityMailboxEvents+4; i++ {
		coord.recordActivity(Event{
			Kind: EventAgentCompleted, SessionID: ref.SessionID, ParentID: ref.TurnID,
			AgentID: fmt.Sprintf("agility-%d", i+1), Profile: ProfileAgility,
		})
	}
	result, err := coord.WaitActivityAfter(context.Background(), ref, 0, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || len(result.Events) != maxActivityMailboxEvents {
		t.Fatalf("result = cursor=%d truncated=%v events=%d", result.Cursor, result.Truncated, len(result.Events))
	}
}
