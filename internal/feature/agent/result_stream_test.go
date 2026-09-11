package agent

import (
	"context"
	"testing"
	"time"
)

func TestResultEventCursorIsConsumerOwned(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	ref := TurnRef{SessionID: "session-a", TurnID: "turn-1"}
	coord.recordResultEvent(Event{
		Kind:          EventAgentResultAvailable,
		SessionID:     ref.SessionID,
		ParentID:      ref.TurnID,
		AgentID:       "agility-1",
		Profile:       ProfileAgility,
		ResultVersion: 3,
	})

	first, err := coord.WaitResultEventsAfter(context.Background(), ref, 0, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	second, err := coord.WaitResultEventsAfter(context.Background(), ref, 0, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 1 || len(second.Events) != 1 {
		t.Fatalf("independent cursors lost result events: first=%+v second=%+v", first, second)
	}
	if first.Cursor != second.Cursor || first.Cursor == 0 {
		t.Fatalf("cursor mismatch: first=%d second=%d", first.Cursor, second.Cursor)
	}
	if first.Events[0].ResultVersion != 3 || second.Events[0].ResultVersion != 3 {
		t.Fatalf("result version mismatch: first=%+v second=%+v", first.Events[0], second.Events[0])
	}

	after, err := coord.WaitResultEventsAfter(context.Background(), ref, first.Cursor, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !after.TimedOut || len(after.Events) != 0 || after.Cursor != first.Cursor {
		t.Fatalf("wait after cursor = %+v, want non-destructive timeout", after)
	}
}

func TestResultEventStreamIsTurnScoped(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	coord.recordResultEvent(Event{Kind: EventAgentResultAvailable, SessionID: "session-a", ParentID: "turn-a", AgentID: "agility-1", ResultVersion: 1})

	batch, err := coord.WaitResultEventsAfter(context.Background(), TurnRef{SessionID: "session-a", TurnID: "turn-b"}, 0, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !batch.TimedOut || len(batch.Events) != 0 {
		t.Fatalf("cross-turn result event leaked: %+v", batch)
	}
}
