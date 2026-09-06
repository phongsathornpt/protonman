package tool

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
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
		"cat README.md > copy.md": CommandEffectMutating,
		"pwd && rm x":             CommandEffectMutating,
		"find . -delete":          CommandEffectMutating,
	}
	for command, want := range tests {
		if got := ClassifyCommandEffect(command); got != want {
			t.Fatalf("ClassifyCommandEffect(%q) = %q, want %q", command, got, want)
		}
	}
}

func TestAnalyzeCommandCompositionAndAffectedPaths(t *testing.T) {
	tests := []struct {
		command string
		effect  CommandEffect
		paths   []string
	}{
		{"pwd && git status --short", CommandEffectReadOnly, nil},
		{"pwd && rm tmp.txt", CommandEffectMutating, []string{"tmp.txt"}},
		{"cat README.md > copy.md", CommandEffectMutating, []string{"copy.md"}},
		{"cp source.txt dest.txt", CommandEffectMutating, []string{"dest.txt"}},
		{"mv old.txt new.txt", CommandEffectMutating, []string{"old.txt", "new.txt"}},
		{`echo x > "$TARGET"`, CommandEffectMutating, nil},
	}
	for _, tt := range tests {
		got := AnalyzeCommand(tt.command)
		if got.Effect != tt.effect {
			t.Fatalf("AnalyzeCommand(%q).Effect = %q, want %q (reason=%s)", tt.command, got.Effect, tt.effect, got.Reason)
		}
		if !slices.Equal(got.AffectedPaths, tt.paths) {
			t.Fatalf("AnalyzeCommand(%q).AffectedPaths = %#v, want %#v", tt.command, got.AffectedPaths, tt.paths)
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

func TestAnalyzeCommandRedirectionAndExpansionSafety(t *testing.T) {
	tests := []struct {
		command string
		effect  CommandEffect
		paths   []string
	}{
		{`git status >/dev/null 2>&1`, CommandEffectReadOnly, nil},
		{`printf x &> out.log`, CommandEffectMutating, []string{"out.log"}},
		{`printf x >| out.log`, CommandEffectMutating, []string{"out.log"}},
		{`printf x 2> err.log`, CommandEffectMutating, []string{"err.log"}},
		{`tee TODO.md`, CommandEffectMutating, []string{"TODO.md"}},
		{`tee`, CommandEffectReadOnly, nil},
		{`cat "$(rm -f x)"`, CommandEffectUnknown, nil},
		{`echo hello`, CommandEffectReadOnly, nil},
		{`printf hello`, CommandEffectReadOnly, nil},
	}
	for _, tt := range tests {
		got := AnalyzeCommand(tt.command)
		if got.Effect != tt.effect {
			t.Fatalf("AnalyzeCommand(%q).Effect = %q, want %q (reason=%s)", tt.command, got.Effect, tt.effect, got.Reason)
		}
		if !slices.Equal(got.AffectedPaths, tt.paths) {
			t.Fatalf("AnalyzeCommand(%q).AffectedPaths = %#v, want %#v", tt.command, got.AffectedPaths, tt.paths)
		}
	}
}

func TestAnalyzeCommandMultipleMoveSources(t *testing.T) {
	got := AnalyzeCommand(`mv a.txt b.txt archive/`)
	if got.Effect != CommandEffectMutating {
		t.Fatalf("effect = %q", got.Effect)
	}
	want := []string{"a.txt", "b.txt", "archive"}
	if !slices.Equal(got.AffectedPaths, want) {
		t.Fatalf("AffectedPaths = %#v, want %#v", got.AffectedPaths, want)
	}
}

func TestAnalyzeCommandRisk(t *testing.T) {
	tests := map[string]CommandRisk{
		"git add file.go":                         CommandRiskNormal,
		"git commit -am fix":                      CommandRiskNormal,
		"touch file.go":                           CommandRiskNormal,
		"rm file.go":                              CommandRiskDestructive,
		"truncate -s 0 file.go":                   CommandRiskDestructive,
		"find . -delete":                          CommandRiskDestructive,
		"git reset --hard HEAD":                   CommandRiskDestructive,
		"git clean -fdx":                          CommandRiskDestructive,
		"git checkout -- file.go":                 CommandRiskDestructive,
		"git restore file.go":                     CommandRiskDestructive,
		"git branch -D old":                       CommandRiskDestructive,
		"git stash drop":                          CommandRiskDestructive,
		"git push --force origin main":            CommandRiskRemoteDestructive,
		"git push --force-with-lease origin main": CommandRiskRemoteDestructive,
		"pwd && git reset --hard HEAD":            CommandRiskDestructive,
		"git push origin main":                    CommandRiskNormal,
	}
	for command, want := range tests {
		if got := AnalyzeCommand(command).Risk; got != want {
			t.Fatalf("AnalyzeCommand(%q).Risk = %q, want %q", command, got, want)
		}
	}
}

func TestAnalyzeCommandScope(t *testing.T) {
	tests := map[string]CommandScope{
		"touch file.go":             CommandScopeLocal,
		"git push origin main":      CommandScopeRemote,
		"git push --force origin x": CommandScopeRemote,
		"npm publish":               CommandScopePublish,
		"pnpm publish":              CommandScopePublish,
		"yarn npm publish":          CommandScopePublish,
		"cargo publish":             CommandScopePublish,
		"wrangler deploy":           CommandScopeDeployment,
		"terraform apply":           CommandScopeDeployment,
		"terraform destroy":         CommandScopeDeployment,
		"kubectl apply -f app.yaml": CommandScopeDeployment,
		"kubectl delete pod api":    CommandScopeDeployment,
	}
	for command, want := range tests {
		if got := AnalyzeCommand(command).Scope; got != want {
			t.Fatalf("AnalyzeCommand(%q).Scope = %q, want %q", command, got, want)
		}
	}
	if got := AnalyzeCommand("terraform destroy").Risk; got != CommandRiskRemoteDestructive {
		t.Fatalf("terraform destroy risk = %q", got)
	}
}
