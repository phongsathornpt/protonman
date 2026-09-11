package agent

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestSynthesisCoordinatorDrainsEachResultOnce(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	turnRef := TurnRef{SessionID: "session-a", TurnID: "turn-1"}
	resultRef := ResultRef{SessionID: turnRef.SessionID, AgentID: "agility-1", Version: 2}
	coord.resultStore.Put(resultRef, Result{
		SessionID:  turnRef.SessionID,
		AgentID:    resultRef.AgentID,
		Profile:    ProfileAgility,
		Conclusion: "found the lifecycle edge",
	})
	event := Event{
		Kind: EventAgentResultAvailable, SessionID: turnRef.SessionID,
		ParentID: turnRef.TurnID, AgentID: resultRef.AgentID,
		Profile: ProfileAgility, ResultVersion: resultRef.Version,
	}
	coord.recordResultEvent(event)
	coord.recordResultEvent(event)

	synthesis := NewSynthesisCoordinator(coord)
	batch, err := synthesis.Drain(context.Background(), turnRef, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 1 {
		t.Fatalf("results = %+v, want one deduplicated result", batch.Results)
	}
	if batch.Results[0].Ref != resultRef || batch.Results[0].Result.Conclusion != "found the lifecycle edge" {
		t.Fatalf("unexpected synthesis result: %+v", batch.Results[0])
	}

	again, err := synthesis.Drain(context.Background(), turnRef, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !again.TimedOut || len(again.Results) != 0 {
		t.Fatalf("second drain replayed consumed results: %+v", again)
	}
}

func TestSynthesisCoordinatorKeepsTurnsIsolated(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil)
	defer coord.Close()
	synthesis := NewSynthesisCoordinator(coord)

	for _, turnID := range []string{"turn-a", "turn-b"} {
		ref := ResultRef{SessionID: "session-a", AgentID: "agility-" + turnID, Version: 1}
		coord.resultStore.Put(ref, Result{SessionID: ref.SessionID, AgentID: ref.AgentID, Profile: ProfileAgility, Conclusion: turnID})
		coord.recordResultEvent(Event{Kind: EventAgentResultAvailable, SessionID: ref.SessionID, ParentID: turnID, AgentID: ref.AgentID, Profile: ProfileAgility, ResultVersion: ref.Version})
	}

	batch, err := synthesis.Drain(context.Background(), TurnRef{SessionID: "session-a", TurnID: "turn-a"}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Results) != 1 || batch.Results[0].Result.Conclusion != "turn-a" {
		t.Fatalf("cross-turn synthesis leak: %+v", batch.Results)
	}
}

func TestSynthesisCoordinatorRecoversWhenResultStreamTruncates(t *testing.T) {
	coord := NewCoordinator(nil, emptyRegistry{}, nil, nil, WithMaxRetainedAgents(512))
	defer coord.Close()
	turnRef := TurnRef{SessionID: "session-a", TurnID: "turn-1"}
	const count = maxActivityMailboxEvents + 12

	coord.agentsMu.Lock()
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("agility-%03d", i)
		ref := ResultRef{SessionID: turnRef.SessionID, AgentID: id, Version: 1}
		result := Result{SessionID: turnRef.SessionID, AgentID: id, Profile: ProfileAgility, Conclusion: id}
		coord.resultStore.Put(ref, result)
		coord.agents[id] = &agentEntry{
			status: AgentStatus{SessionID: turnRef.SessionID, ID: id, ParentID: turnRef.TurnID, Profile: ProfileAgility, Task: "inspect", State: StateCompleted, Version: 1},
			result: result, resultRef: ref,
		}
	}
	coord.agentsMu.Unlock()

	for i := 0; i < count; i++ {
		id := fmt.Sprintf("agility-%03d", i)
		coord.recordResultEvent(Event{
			Kind: EventAgentResultAvailable, SessionID: turnRef.SessionID,
			ParentID: turnRef.TurnID, AgentID: id, Profile: ProfileAgility, ResultVersion: 1,
		})
	}

	batch, err := NewSynthesisCoordinator(coord).Drain(context.Background(), turnRef, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !batch.Truncated {
		t.Fatal("batch did not report truncated stream")
	}
	if len(batch.Results) != count {
		t.Fatalf("recovered results = %d, want %d", len(batch.Results), count)
	}
}
