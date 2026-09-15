package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestNewCallCopiesArguments(t *testing.T) {
	arguments := []byte(`{"path":"README.md"}`)
	call, err := NewCall("call-1", "read", arguments)
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	arguments[0] = ' '
	if string(call.Arguments) != `{"path":"README.md"}` {
		t.Fatalf("call arguments changed after source mutation: %s", call.Arguments)
	}
}

func TestNewCallDefaultsEmptyArguments(t *testing.T) {
	call, err := NewCall("call-1", "read", nil)
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
		{
			name:      "filesystem not found",
			err:       fmt.Errorf("open target: %w", os.ErrNotExist),
			wantCode:  ErrorCodeNotFound,
			wantRetry: false,
		},
		{
			name:      "filesystem permission denied",
			err:       fmt.Errorf("open target: %w", os.ErrPermission),
			wantCode:  ErrorCodePermissionDenied,
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
		ToolName: "read",
		Failure:  &Failure{Code: ErrorCodeNotFound, Message: "missing"},
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(encoded), `{"call_id":"call-1","tool_name":"read","error":{"code":"not_found","message":"missing"}}`; got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestResultContinuationHasStableJSONShape(t *testing.T) {
	next := int64(42)
	result := Result{
		CallID:     "call-1",
		ToolName:   "read",
		Output:     "page",
		Truncated:  true,
		NextOffset: &next,
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(encoded), `{"call_id":"call-1","tool_name":"read","output":"page","truncated":true,"next_offset":42}`; got != want {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}

func TestDefinitionValidateRejectsUnknownEvidence(t *testing.T) {
	err := (Definition{
		Name:        "broken",
		Description: "bad evidence",
		Kind:        KindRead,
		Evidence:    EvidenceKind("perhaps"),
	}).Validate()
	if err == nil {
		t.Fatal("Definition.Validate() error = nil, want evidence error")
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
	result := Result{CallID: "call-1", ToolName: "read", Output: "page", Truncated: true, NextOffset: &next, Continuation: "abc123"}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(encoded), `{"call_id":"call-1","tool_name":"read","output":"page","truncated":true,"next_offset":42,"continuation":"abc123"}`; got != want {
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

func TestAnalyzeCommandMarksConflictProneGitOperations(t *testing.T) {
	for _, command := range []string{
		"git merge feature",
		"git rebase main",
		"git cherry-pick deadbeef",
		"git apply change.patch",
		"pwd && git merge feature",
	} {
		if !AnalyzeCommand(command).ConflictProne {
			t.Fatalf("AnalyzeCommand(%q).ConflictProne = false", command)
		}
	}
	for _, command := range []string{"git status", "git add file.go", "git commit -m ok", "git push origin main"} {
		if AnalyzeCommand(command).ConflictProne {
			t.Fatalf("AnalyzeCommand(%q).ConflictProne = true", command)
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

func TestNoArgumentsSchemaDetection(t *testing.T) {
	if !IsNoArgumentsSchema(NoArgumentsSchema()) {
		t.Fatal("canonical no-arguments schema was not detected")
	}
	if IsNoArgumentsSchema(map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}, "additionalProperties": false}) {
		t.Fatal("schema with properties detected as no-arguments")
	}
}

func TestNormalizeArgumentsDropsObjectMetadataForNoArgumentTools(t *testing.T) {
	definition := Definition{Name: "zero", Description: "zero args", Kind: KindRead, InputSchema: NoArgumentsSchema()}
	for _, raw := range []string{`{}`, `{"reason":"checking tasks"}`, `{"query":"running","description":"inspect"}`} {
		if got := string(NormalizeArguments(definition, json.RawMessage(raw))); got != `{}` {
			t.Fatalf("NormalizeArguments(%s) = %s, want {}", raw, got)
		}
	}
	for _, raw := range []string{`[]`, `"metadata"`, `1`, `true`} {
		if got := string(NormalizeArguments(definition, json.RawMessage(raw))); got != raw {
			t.Fatalf("NormalizeArguments(%s) = %s, want unchanged", raw, got)
		}
	}
}

func TestNormalizeArgumentsCanonicalizesInputAliases(t *testing.T) {
	definition := Definition{Name: "ls", Description: "list", Kind: KindRead, InputSchema: map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "additionalProperties": false}, InputAliases: map[string][]string{"path": {"dir_path", "directory"}}}
	for _, raw := range []string{`{"dir_path":"cmd"}`, `{"directory":"cmd"}`, `{"path":"cmd","dir_path":"cmd"}`} {
		got := NormalizeArguments(definition, json.RawMessage(raw))
		var values map[string]any
		if err := json.Unmarshal(got, &values); err != nil {
			t.Fatalf("NormalizeArguments(%s) invalid JSON: %v", raw, err)
		}
		if values["path"] != "cmd" || len(values) != 1 {
			t.Fatalf("NormalizeArguments(%s) = %s, want canonical path only", raw, got)
		}
	}
	conflict := `{"path":"cmd","dir_path":"internal"}`
	if got := string(NormalizeArguments(definition, json.RawMessage(conflict))); got != conflict {
		t.Fatalf("conflicting aliases normalized to %s, want unchanged for schema rejection", got)
	}
}

// A tool whose handler dispatches case-insensitively must not publish a schema
// that rejects the case variant the handler would have accepted.
func TestNormalizeArgumentsFoldsRootEnumCase(t *testing.T) {
	definition := Definition{
		Name: "edit", Description: "edit", Kind: KindEdit,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":    map[string]any{"type": "string", "enum": []string{"write", "replace"}},
				"file_path": map[string]any{"type": "string"},
			},
			"required": []any{"action"}, "additionalProperties": false,
		},
	}
	for raw, want := range map[string]string{
		`{"action":"WRITE","file_path":"a.go"}`: `"action":"write"`,
		`{"action":"Replace"}`:                  `"action":"replace"`,
		`{"action":"  write  "}`:                `"action":"write"`,
	} {
		got := string(NormalizeArguments(definition, json.RawMessage(raw)))
		if !strings.Contains(got, want) {
			t.Fatalf("NormalizeArguments(%s) = %s, want %s", raw, got, want)
		}
	}
	// A misspelling is not silently repaired; the schema still reports it.
	for _, raw := range []string{`{"action":"wrote"}`, `{"action":"WRITE!"}`, `{"action":1}`} {
		if got := string(NormalizeArguments(definition, json.RawMessage(raw))); got != raw {
			t.Fatalf("NormalizeArguments(%s) = %s, want unchanged for schema rejection", raw, got)
		}
	}
}

// Folding must be scoped to enum fields: free-form string arguments such as a
// grep pattern or file path keep their exact case.
func TestNormalizeArgumentsLeavesFreeFormStringsAlone(t *testing.T) {
	definition := Definition{
		Name: "grep", Description: "grep", Kind: KindGrep,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":  map[string]any{"type": "string", "enum": []string{"search"}},
				"pattern": map[string]any{"type": "string"},
				"path":    map[string]any{"type": "string"},
			},
			"required": []any{"action"}, "additionalProperties": false,
		},
	}
	raw := `{"action":"SEARCH","pattern":"TODO: FixMe","path":"Src/PKG"}`
	got := string(NormalizeArguments(definition, json.RawMessage(raw)))
	if !strings.Contains(got, `"action":"search"`) {
		t.Fatalf("action was not folded: %s", got)
	}
	if !strings.Contains(got, "TODO: FixMe") || !strings.Contains(got, "Src/PKG") {
		t.Fatalf("free-form strings lost case: %s", got)
	}
}

// Enums that are not lowercase-simple are skipped entirely, so unexpected
// vocabulary is never case-folded.
func TestNormalizeArgumentsSkipsNonSimpleEnums(t *testing.T) {
	for name, enum := range map[string]any{
		"mixed case":  []string{"ReadOnly", "readwrite"},
		"non-string":  []any{1, 2},
		"empty entry": []string{""},
	} {
		definition := Definition{
			Name: "custom", Description: "custom", Kind: KindRead,
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"action": map[string]any{"type": "string", "enum": enum}},
			},
		}
		raw := `{"action":"READONLY"}`
		if got := string(NormalizeArguments(definition, json.RawMessage(raw))); got != raw {
			t.Fatalf("%s: NormalizeArguments(%s) = %s, want unchanged", name, raw, got)
		}
	}
}

// JSON Schema accepts 3.0 and 1e2 as integers, but encoding/json cannot decode
// either into a Go int, so a schema-valid call would fail inside the handler.
func TestNormalizeArgumentsCanonicalizesIntegralNumbers(t *testing.T) {
	definition := Definition{
		Name: "ls", Description: "list", Kind: KindRead,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "minimum": 0},
				"offset": map[string]any{"type": "integer", "minimum": 0},
				"path":   map[string]any{"type": "string"},
			},
			"additionalProperties": false,
		},
	}
	for raw, want := range map[string]string{
		`{"limit":3.0}`:              `"limit":3`,
		`{"limit":100.00}`:           `"limit":100`,
		`{"limit":1e2}`:              `"limit":100`,
		`{"limit":-5.0}`:             `"limit":-5`,
		`{"offset":0.0}`:             `"offset":0`,
		`{"limit":3.0,"offset":2.0}`: `"limit":3`,
	} {
		got := string(NormalizeArguments(definition, json.RawMessage(raw)))
		if !strings.Contains(got, want) {
			t.Fatalf("NormalizeArguments(%s) = %s, want it to contain %s", raw, got, want)
		}
	}
	// Already-integral and genuinely wrong values keep their spelling so the
	// schema can judge them.
	for _, raw := range []string{
		`{"limit":3}`,
		`{"limit":3.5}`,
		`{"limit":"3"}`,
		`{"limit":true}`,
		`{"path":"a.go"}`,
	} {
		if got := string(NormalizeArguments(definition, json.RawMessage(raw))); got != raw {
			t.Fatalf("NormalizeArguments(%s) = %s, want unchanged", raw, got)
		}
	}
}

// Numeric folding is scoped to integer fields; a float-typed or string-typed
// field must not be rewritten.
func TestNormalizeArgumentsLeavesNonIntegerFieldsAlone(t *testing.T) {
	definition := Definition{
		Name: "threshold", Description: "threshold", Kind: KindRead,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ratio": map[string]any{"type": "number"},
				"name":  map[string]any{"type": "string"},
			},
			"additionalProperties": false,
		},
	}
	raw := `{"ratio":3.0,"name":"a"}`
	if got := string(NormalizeArguments(definition, json.RawMessage(raw))); got != raw {
		t.Fatalf("NormalizeArguments(%s) = %s, want unchanged", raw, got)
	}
}

func TestNewCallNormalizesBlankArguments(t *testing.T) {
	for _, raw := range []string{"", "   ", "\n\t"} {
		call, err := NewCall("call-1", "todo", []byte(raw))
		if err != nil {
			t.Fatalf("NewCall(%q) error = %v", raw, err)
		}
		if got := string(call.Arguments); got != `{}` {
			t.Fatalf("NewCall(%q) arguments = %q, want {}", raw, got)
		}
	}
}

func TestResultModelPayloadDropsRedundantStreams(t *testing.T) {
	original := Result{
		CallID: "call-1", ToolName: "bash", Output: "stdout\nstderr",
		Stdout: "stdout", Stderr: "stderr", StdoutBytes: 6, StderrBytes: 6,
		StdoutTruncated: true, Truncated: true,
	}
	payload := original.ModelPayload()
	if payload.Stdout != "" || payload.Stderr != "" {
		t.Fatalf("model payload retained duplicate streams: %#v", payload)
	}
	if payload.Output != original.Output || payload.StdoutBytes != 6 || !payload.StdoutTruncated || !payload.Truncated {
		t.Fatalf("model payload lost stream metadata: %#v", payload)
	}
	if original.Stdout != "stdout" || original.Stderr != "stderr" {
		t.Fatalf("ModelPayload mutated original result: %#v", original)
	}
}

func TestResultModelPayloadKeepsStreamsWithoutCompatibilityOutput(t *testing.T) {
	original := Result{ToolName: "custom", Stdout: "stdout", Stderr: "stderr"}
	payload := original.ModelPayload()
	if payload.Stdout != "stdout" || payload.Stderr != "stderr" {
		t.Fatalf("model payload dropped sole stream content: %#v", payload)
	}
}

func TestResultModelPayloadCompactsImageData(t *testing.T) {
	original := Result{
		CallID:   "call-1",
		ToolName: "read",
		Output:   "image summary",
		Image: &ImageAttachment{
			MIMEType: "image/png",
			Data:     "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
			Width:    100,
			Height:   100,
		},
	}
	payload := original.ModelPayload()
	if payload.Image == nil {
		t.Fatal("expected image attachment in payload")
	}
	if payload.Image.Data != "" {
		t.Fatalf("expected Image.Data to be stripped in ModelPayload, got %q", payload.Image.Data)
	}
	if payload.Image.MIMEType != "image/png" || payload.Image.Width != 100 || payload.Image.Height != 100 {
		t.Fatalf("unexpected image metadata in ModelPayload: %+v", payload.Image)
	}
	// Verify original was not mutated
	if original.Image.Data == "" {
		t.Fatal("ModelPayload mutated original Result.Image.Data")
	}
}

func TestFailureFromErrorUsesSemanticToolMessage(t *testing.T) {
	err := fmt.Errorf("execute read: %w", WrapToolError(ErrorCodeNotFound, `not found: "src/missing.go"`, os.ErrNotExist))
	failure := FailureFromError(err)
	if failure == nil {
		t.Fatal("FailureFromError() = nil")
	}
	if failure.Message != `not found: "src/missing.go"` {
		t.Fatalf("failure message = %q", failure.Message)
	}
	if strings.Contains(failure.Message, "execute read") || strings.Contains(failure.Message, os.ErrNotExist.Error()) {
		t.Fatalf("failure leaked wrapper/cause: %q", failure.Message)
	}
}

func TestResultModelPayloadCompactsFailureMessage(t *testing.T) {
	message := "  first line\n\t" + strings.Repeat("x", 300)
	original := Result{ToolName: "read", Failure: &Failure{Code: ErrorCodeNotFound, Message: message}}
	payload := original.ModelPayload()
	if payload.Failure == original.Failure {
		t.Fatal("ModelPayload reused failure pointer")
	}
	if strings.Contains(payload.Failure.Message, "\n") || len([]rune(payload.Failure.Message)) > maxModelFailureMessageChars {
		t.Fatalf("model failure was not compacted: %q", payload.Failure.Message)
	}
	if original.Failure.Message != message {
		t.Fatal("ModelPayload mutated original failure")
	}
}

func TestFailureFromErrorPreservesExplicitDiagnostic(t *testing.T) {
	cause := errors.New("todo operation 2: set_status does not accept text")
	err := fmt.Errorf("execute todo: %w", WrapToolError(ErrorCodeInvalidArguments, "apply todo patch", cause).WithDiagnostic(cause.Error()))
	failure := FailureFromError(err)
	if failure == nil {
		t.Fatal("FailureFromError() = nil")
	}
	if failure.Message != "apply todo patch" {
		t.Fatalf("failure message = %q", failure.Message)
	}
	if failure.Diagnostic != cause.Error() {
		t.Fatalf("failure diagnostic = %q, want %q", failure.Diagnostic, cause.Error())
	}
	if strings.Contains(failure.Diagnostic, "execute todo") {
		t.Fatalf("failure diagnostic leaked wrapper: %q", failure.Diagnostic)
	}
}

// A handler's decode failure names no defect on its own ("decode ls arguments").
// The wrapped cause must be surfaced so the model can see which field was wrong.
func TestFailureFromErrorSurfacesInvalidArgumentCause(t *testing.T) {
	cause := errors.New("json: cannot unmarshal string into Go struct field listDirInput.limit of type int")
	err := WrapToolError(ErrorCodeInvalidArguments, "decode ls arguments", cause)
	failure := FailureFromError(err)
	if failure == nil {
		t.Fatal("FailureFromError() = nil")
	}
	if failure.Message != "decode ls arguments" {
		t.Fatalf("failure message = %q", failure.Message)
	}
	if failure.Diagnostic != cause.Error() {
		t.Fatalf("failure diagnostic = %q, want the wrapped cause", failure.Diagnostic)
	}
}

// An explicit diagnostic still wins over the wrapped cause, and non-caller-fixable
// codes are left alone.
func TestFailureFromErrorDiagnosticPrecedence(t *testing.T) {
	explicit := WrapToolError(ErrorCodeInvalidArguments, "msg", errors.New("cause")).WithDiagnostic("explicit detail")
	if got := FailureFromError(explicit).Diagnostic; got != "explicit detail" {
		t.Fatalf("diagnostic = %q, want explicit detail", got)
	}
	execution := WrapToolError(ErrorCodeExecution, "run command", errors.New("exit status 1"))
	if got := FailureFromError(execution).Diagnostic; got != "" {
		t.Fatalf("execution diagnostic = %q, want empty", got)
	}
}

func TestResultModelPayloadCompactsFailureDiagnostic(t *testing.T) {
	diagnostic := "  detail\n\t" + strings.Repeat("x", 300)
	original := Result{ToolName: "todo", Failure: &Failure{Code: ErrorCodeInvalidArguments, Message: "invalid task patch", Diagnostic: diagnostic}}
	payload := original.ModelPayload()
	if payload.Failure == original.Failure {
		t.Fatal("ModelPayload reused failure pointer")
	}
	if strings.Contains(payload.Failure.Diagnostic, "\n") || len([]rune(payload.Failure.Diagnostic)) > maxModelFailureMessageChars {
		t.Fatalf("model failure diagnostic was not compacted: %q", payload.Failure.Diagnostic)
	}
	if original.Failure.Diagnostic != diagnostic {
		t.Fatal("ModelPayload mutated original failure diagnostic")
	}
}
