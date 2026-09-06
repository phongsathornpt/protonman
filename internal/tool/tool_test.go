package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestNewCallCopiesArguments(t *testing.T) {
	arguments := []byte(`{"path":"README.md"}`)
	call, err := NewCall("call-1", "read_file", arguments)
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	arguments[0] = ' '
	if string(call.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("call arguments changed after source mutation: %s", call.Arguments)
	}
}

func TestNewCallDefaultsEmptyArguments(t *testing.T) {
	call, err := NewCall("call-1", "read_file", nil)
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	if string(call.Arguments) != `{}` {
		t.Fatalf("call arguments = %s, want {}", call.Arguments)
	}
}

func TestDefinitionValidateRejectsUnknownKind(t *testing.T) {
	err := (Definition{
		Name:        "broken",
		Description: "missing kind",
		Kind:        Kind("unknown"),
	}).Validate()
	if err == nil {
		t.Fatal("Definition.Validate() error = nil, want error")
	}
}

func TestFailureFromErrorClassifiesStableCodes(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantCode  ErrorCode
		wantRetry bool
	}{
		{
			name:      "invalid call",
			err:       ErrInvalidCall,
			wantCode:  ErrorCodeInvalidArguments,
			wantRetry: false,
		},
		{
			name:      "canceled",
			err:       context.Canceled,
			wantCode:  ErrorCodeCanceled,
			wantRetry: true,
		},
		{
			name:      "coded error",
			err:       WrapToolError(ErrorCodeNotFound, "missing file", errors.New("no entry")),
			wantCode:  ErrorCodeNotFound,
			wantRetry: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := FailureFromError(test.err)
			if failure == nil {
				t.Fatal("FailureFromError() = nil")
			}
			if failure.Code != test.wantCode {
				t.Fatalf("failure code = %q, want %q", failure.Code, test.wantCode)
			}
			if failure.Retryable != test.wantRetry {
				t.Fatalf("failure retryable = %t, want %t", failure.Retryable, test.wantRetry)
			}
		})
	}
}

func TestResultFailureHasStableJSONShape(t *testing.T) {
	result := Result{
		CallID:   "call-1",
		ToolName: "read_file",
		Failure:  &Failure{Code: ErrorCodeNotFound, Message: "missing"},
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(encoded), `{"call_id":"call-1","tool_name":"read_file","error":{"code":"not_found","message":"missing"}}`; got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestResultContinuationHasStableJSONShape(t *testing.T) {
	next := int64(42)
	result := Result{
		CallID:     "call-1",
		ToolName:   "read_file",
		Output:     "page",
		Truncated:  true,
		NextOffset: &next,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(encoded), `{"call_id":"call-1","tool_name":"read_file","output":"page","truncated":true,"next_offset":42}`; got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestDefinitionValidateRejectsUnknownMutability(t *testing.T) {
	err := (Definition{
		Name:        "broken",
		Description: "bad mutability",
		Kind:        KindRead,
		Mutability:  Mutability("sometimes"),
	}).Validate()
	if err == nil {
		t.Fatal("Definition.Validate() error = nil, want mutability error")
	}
}

func TestEffectiveMutabilityPrefersExplicitMetadata(t *testing.T) {
	definition := Definition{Kind: KindBash, Mutability: MutabilityReadOnly}
	if got := EffectiveMutability(definition); got != MutabilityReadOnly {
		t.Fatalf("EffectiveMutability() = %q, want read_only", got)
	}
	legacy := Definition{Kind: KindRead}
	if got := EffectiveMutability(legacy); got != MutabilityReadOnly {
		t.Fatalf("legacy read mutability = %q, want read_only", got)
	}
}

func TestClassifyCommandEffect(t *testing.T) {
	tests := map[string]CommandEffect{
		"pwd":                     CommandEffectReadOnly,
		"git status --short":      CommandEffectReadOnly,
		"git diff -- README.md":   CommandEffectReadOnly,
		"git branch":              CommandEffectReadOnly,
		"git branch --list":       CommandEffectReadOnly,
		"git branch feature/x":    CommandEffectMutating,
		"git branch -D old":       CommandEffectMutating,
		"find . -name '*.go'":     CommandEffectReadOnly,
		"rm -rf tmp":              CommandEffectMutating,
		"git clean -fdx":          CommandEffectMutating,
		"cat README.md > copy.md": CommandEffectUnknown,
		"pwd && rm x":             CommandEffectUnknown,
		"find . -delete":          CommandEffectUnknown,
	}
	for command, want := range tests {
		if got := ClassifyCommandEffect(command); got != want {
			t.Fatalf("ClassifyCommandEffect(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestEffectiveCallMutabilityRefinesBash(t *testing.T) {
	definition := Definition{Kind: KindBash, Mutability: MutabilityMutating}
	if got := EffectiveCallMutability(definition, json.RawMessage(`{"command":"git status"}`)); got != MutabilityReadOnly {
		t.Fatalf("read-only bash mutability = %q", got)
	}
	if got := EffectiveCallMutability(definition, json.RawMessage(`{"command":"git clean -fdx"}`)); got != MutabilityMutating {
		t.Fatalf("mutating bash mutability = %q", got)
	}
}

func TestResultSnapshotContinuationHasStableJSONShape(t *testing.T) {
	next := int64(42)
	result := Result{CallID: "call-1", ToolName: "read_file", Output: "page", Truncated: true, NextOffset: &next, Continuation: "abc123"}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(encoded), `{"call_id":"call-1","tool_name":"read_file","output":"page","truncated":true,"next_offset":42,"continuation":"abc123"}`; got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}
