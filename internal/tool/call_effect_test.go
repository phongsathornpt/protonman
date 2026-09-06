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
