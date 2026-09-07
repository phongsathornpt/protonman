package turn

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/projectTHORN/proton/internal/core/tool"
)

func TestVerificationStateTracksMutationAndVerifierOrder(t *testing.T) {
	defs := []tool.Definition{
		{Name: "write_file", Kind: tool.KindEdit, Mutability: tool.MutabilityMutating},
		{Name: "bash", Kind: tool.KindBash, Mutability: tool.MutabilityMutating},
	}
	state := VerificationState{}
	state.observe([]executedCall{successfulBashCall(t, "go test ./...")}, defs)
	if state.Mutated || state.Verified {
		t.Fatalf("pre-mutation verifier changed state: %#v", state)
	}
	state.observe([]executedCall{successfulToolCall(t, "write_file", `{}`)}, defs)
	if !state.Mutated || state.Verified {
		t.Fatalf("mutation state = %#v, want mutated/unverified", state)
	}
	state.observe([]executedCall{successfulBashCall(t, "go test ./...")}, defs)
	if !state.Verified || state.Verifier != "go test" {
		t.Fatalf("verified state = %#v", state)
	}
}
func TestVerificationStateFailedVerifierAndLaterMutationReset(t *testing.T) {
	defs := []tool.Definition{
		{Name: "write_file", Kind: tool.KindEdit, Mutability: tool.MutabilityMutating},
		{Name: "bash", Kind: tool.KindBash, Mutability: tool.MutabilityMutating},
	}
	state := VerificationState{}
	state.observe([]executedCall{successfulToolCall(t, "write_file", `{}`)}, defs)
	failed := successfulBashCall(t, "go test ./...")
	failed.err = errors.New("tests failed")
	state.observe([]executedCall{failed}, defs)
	if state.Verified {
		t.Fatalf("failed verifier marked state verified: %#v", state)
	}
	state.observe([]executedCall{successfulBashCall(t, "git diff --check")}, defs)
	if !state.Verified {
		t.Fatalf("successful verifier did not verify: %#v", state)
	}
	state.observe([]executedCall{successfulToolCall(t, "write_file", `{}`)}, defs)
	if state.Verified || state.Verifier != "" {
		t.Fatalf("later mutation did not reset verifier: %#v", state)
	}
}

func successfulBashCall(t *testing.T, command string) executedCall {
	t.Helper()
	return successfulToolCall(t, "bash", `{"command":`+mustJSONQuote(t, command)+`}`)
}
func successfulToolCall(t *testing.T, name, args string) executedCall {
	t.Helper()
	call, err := tool.NewCall("call-"+name, name, json.RawMessage(args))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	return executedCall{
		call:   call,
		result: tool.Result{CallID: call.ID, ToolName: call.Name},
	}
}

func mustJSONQuote(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	return string(encoded)
}
