package turn

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestProgressSafetyBudgetResetsStagnationOnMeaningfulProgress(t *testing.T) {
	definitions := map[string]tool.Definition{
		"todo": {Name: "todo", Kind: tool.KindTask, Mutability: tool.MutabilityMutating, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState}},
		"read": {Name: "read", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
	}
	budget := newProgressSafetyBudget(2, 10)
	if budget.observeRound([]executedCall{{call: tool.Call{Name: "todo", Arguments: json.RawMessage(`{"action":"update"}`)}}}, definitions) {
		t.Fatal("task metadata counted as meaningful progress")
	}
	if budget.stagnantCalls != 1 {
		t.Fatalf("stagnant calls = %d, want 1", budget.stagnantCalls)
	}
	if !budget.observeRound([]executedCall{{call: tool.Call{Name: "read", Arguments: json.RawMessage(`{"path":"a.go"}`)}}}, definitions) {
		t.Fatal("successful repository read did not count as progress")
	}
	if budget.stagnantCalls != 0 || budget.exhausted() {
		t.Fatalf("budget after progress = stagnant:%d exhausted:%v", budget.stagnantCalls, budget.exhausted())
	}
}

func TestProgressSafetyBudgetBoundsStagnantAndEmergencyCalls(t *testing.T) {
	definitions := map[string]tool.Definition{
		"todo": {Name: "todo", Kind: tool.KindTask, Mutability: tool.MutabilityReadOnly, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState}},
		"read": {Name: "read", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
	}
	stagnant := newProgressSafetyBudget(2, 10)
	stagnant.observeRound([]executedCall{{call: tool.Call{Name: "todo", Arguments: json.RawMessage(`{"action":"get"}`)}}}, definitions)
	stagnant.observeRound([]executedCall{{call: tool.Call{Name: "todo", Arguments: json.RawMessage(`{"action":"get"}`)}}}, definitions)
	if !stagnant.exhausted() || stagnant.exhaustedReason() != "stagnant_tool_budget" {
		t.Fatalf("stagnant budget = exhausted:%v reason:%q", stagnant.exhausted(), stagnant.exhaustedReason())
	}

	emergency := newProgressSafetyBudget(100, 2)
	for _, path := range []string{"a.go", "b.go"} {
		emergency.observeRound([]executedCall{{call: tool.Call{Name: "read", Arguments: json.RawMessage(`{"path":"` + path + `"}`)}}}, definitions)
	}
	if !emergency.exhausted() || emergency.exhaustedReason() != "emergency_tool_ceiling" {
		t.Fatalf("emergency budget = exhausted:%v reason:%q", emergency.exhausted(), emergency.exhaustedReason())
	}
}

func TestProgressSafetyBudgetWarningTracksCurrentStagnantWindow(t *testing.T) {
	definitions := map[string]tool.Definition{
		"todo": {Name: "todo", Kind: tool.KindTask, Mutability: tool.MutabilityReadOnly, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState}},
		"read": {Name: "read", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
	}
	budget := newProgressSafetyBudget(10, 100)
	for range 6 {
		budget.observeRound([]executedCall{{call: tool.Call{Name: "todo", Arguments: json.RawMessage(`{"action":"get"}`)}}}, definitions)
	}
	if !budget.shouldWarn(false) || budget.shouldWarn(true) {
		t.Fatalf("warning state = warn:%v already_warned:%v", budget.shouldWarn(false), budget.shouldWarn(true))
	}
	budget.observeRound([]executedCall{{call: tool.Call{Name: "read", Arguments: json.RawMessage(`{"path":"fresh.go"}`)}}}, definitions)
	if budget.shouldWarn(false) {
		t.Fatal("warning did not reset after meaningful progress")
	}
}
