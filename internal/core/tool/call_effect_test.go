package tool

import (
	"encoding/json"
	"testing"
)

func TestEffectiveCallEffect(t *testing.T) {
	bash := Definition{Kind: KindBash, Mutability: MutabilityMutating}
	tests := []struct {
		args string
		want CommandEffect
	}{
		{`{"command":"git status --short"}`, CommandEffectReadOnly},
		{`{"command":"touch file.go"}`, CommandEffectMutating},
		{`{"command":"python3 script.py"}`, CommandEffectUnknown},
		{`{`, CommandEffectUnknown},
	}
	for _, tt := range tests {
		if got := EffectiveCallEffect(bash, json.RawMessage(tt.args)); got != tt.want {
			t.Fatalf("EffectiveCallEffect(%s) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

func TestEffectiveCallSemanticsUsesDefinitionResolver(t *testing.T) {
	definition := Definition{
		Name:       "todo",
		Kind:       KindTask,
		Mutability: MutabilityMutating,
		Safety: SafetyContract{
			MutationDomain: MutationDomainTaskState, MutationSafety: MutationSafetyNone,
			CheckpointPolicy: CheckpointPolicyNone, Boundary: BoundaryPolicyNone,
		},
		Semantics: func(arguments json.RawMessage) CallSemantics {
			semantics := StaticCallSemantics(Definition{
				Kind:       KindTask,
				Mutability: MutabilityMutating,
				Safety: SafetyContract{
					MutationDomain: MutationDomainTaskState, MutationSafety: MutationSafetyNone,
					CheckpointPolicy: CheckpointPolicyNone, Boundary: BoundaryPolicyNone,
				},
			})
			var input struct {
				Action string `json:"action"`
			}
			if json.Unmarshal(arguments, &input) == nil && input.Action == "get" {
				semantics.Mutability = MutabilityReadOnly
				semantics.Effect = CommandEffectReadOnly
			}
			return semantics
		},
	}
	if got := EffectiveCallMutability(definition, json.RawMessage(`{"action":"get"}`)); got != MutabilityReadOnly {
		t.Fatalf("get mutability = %q, want read_only", got)
	}
	if got := EffectiveCallEffect(definition, json.RawMessage(`{"action":"get"}`)); got != CommandEffectReadOnly {
		t.Fatalf("get effect = %q, want read_only", got)
	}
	if got := EffectiveCallMutability(definition, json.RawMessage(`{"action":"update"}`)); got != MutabilityMutating {
		t.Fatalf("update mutability = %q, want mutating", got)
	}
}
