package turn

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestProgressGuardEmptyRoundResetsSynthesisEscalation(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "read", Kind: tool.KindRead}}, 2)
	call := tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)}

	first := executedCall{call: call, result: tool.Result{CallID: call.ID, ToolName: call.Name, Output: "same"}}
	if escalated, err := guard.observeRound([]executedCall{first}); err != nil || escalated {
		t.Fatalf("first read escalated=%v err=%v", escalated, err)
	}

	call.ID = "r2"
	second := executedCall{call: call, result: tool.Result{CallID: call.ID, ToolName: call.Name, Output: "same"}}
	if escalated, err := guard.observeRound([]executedCall{second}); err != nil || escalated {
		t.Fatalf("second read escalated=%v err=%v", escalated, err)
	}
	if got, want := guard.stalledRoundCount(), 1; got != want {
		t.Fatalf("stalled rounds = %d, want %d before reset", got, want)
	}

	// A round without tool executions is not another stalled tool round. This
	// matters when final synthesis is deferred while runtime context is being
	// integrated between tool-using rounds.
	if escalated, err := guard.observeRound(nil); err != nil || escalated {
		t.Fatalf("empty round escalated=%v err=%v", escalated, err)
	}
	if got := guard.stalledRoundCount(); got != 0 {
		t.Fatalf("stalled rounds = %d after empty round, want 0", got)
	}

	call.ID = "r3"
	suppressed, err := guard.suppress(call)
	if err != nil || suppressed == nil {
		t.Fatalf("suppressed read = %#v err=%v", suppressed, err)
	}
	if escalated, err := guard.observeRound([]executedCall{*suppressed}); err != nil || escalated {
		t.Fatalf("stall after empty-round reset escalated=%v err=%v", escalated, err)
	}
	if got, want := guard.stalledRoundCount(), 1; got != want {
		t.Fatalf("stalled rounds = %d, want %d after reset", got, want)
	}
}
