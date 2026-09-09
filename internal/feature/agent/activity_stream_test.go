package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/tool"
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

func TestPruneActivityMailboxesBoundsInactiveTurns(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	now := time.Now()
	coord.activityMu.Lock()
	for i := 0; i < maxActivityMailboxes+12; i++ {
		scope := activityScopeKey(TurnRef{SessionID: "session-a", TurnID: fmt.Sprintf("turn-%d", i)})
		coord.activityMailboxes[scope] = &activityMailbox{notify: make(chan struct{}), updatedAt: now.Add(time.Duration(i) * time.Second)}
	}
	coord.activityMu.Unlock()
	coord.pruneActivityMailboxes(now.Add(time.Minute))
	coord.activityMu.Lock()
	count := len(coord.activityMailboxes)
	coord.activityMu.Unlock()
	if count > maxActivityMailboxes {
		t.Fatalf("mailboxes = %d, want <= %d", count, maxActivityMailboxes)
	}
}

func TestPruneActivityMailboxesExpiresInactiveTurn(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	now := time.Now()
	scope := activityScopeKey(TurnRef{SessionID: "session-a", TurnID: "old-turn"})
	coord.activityMu.Lock()
	coord.activityMailboxes[scope] = &activityMailbox{notify: make(chan struct{}), updatedAt: now.Add(-activityMailboxTTL - time.Second)}
	coord.activityMu.Unlock()
	coord.pruneActivityMailboxes(now)
	coord.activityMu.Lock()
	_, exists := coord.activityMailboxes[scope]
	coord.activityMu.Unlock()
	if exists {
		t.Fatal("expired inactive mailbox was retained")
	}
}

func TestRecordActivityCompactsRetainedPayload(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	call, err := tool.NewCall("call-1", "read", []byte(`{"payload":"`+strings.Repeat("x", 32*1024)+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	coord.recordActivity(Event{
		Kind: EventAgentFailed, AgentID: "agent-1", ParentID: "turn-1",
		Message: strings.Repeat("m", runtimepolicy.AgentActivityMessageBytes+1024),
		Call:    &call, Err: errors.New(strings.Repeat("e", runtimepolicy.AgentActivityErrorBytes+1024)),
	})
	result, err := coord.WaitActivityForTurn(context.Background(), TurnRef{TurnID: "turn-1"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if result.Event == nil {
		t.Fatal("expected retained terminal activity")
	}
	if len(result.Event.Message) > runtimepolicy.AgentActivityMessageBytes {
		t.Fatalf("message bytes=%d", len(result.Event.Message))
	}
	if result.Event.Call == nil || len(result.Event.Call.Arguments) != 0 {
		t.Fatalf("retained call payload = %#v", result.Event.Call)
	}
	if result.Event.Err == nil || len(result.Event.Err.Error()) > runtimepolicy.AgentActivityErrorBytes {
		t.Fatalf("retained error=%v", result.Event.Err)
	}
}
