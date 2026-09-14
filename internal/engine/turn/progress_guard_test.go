package turn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestLoopForcesSynthesisAfterSustainedNoProgressRead(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: repeatedReadEvents("read-1")},
		{events: repeatedReadEvents("read-2")},
		{events: repeatedReadEvents("read-3")},
		{events: repeatedReadEvents("read-4")},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "I already have the file contents."},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow)

	result, err := loop.Run(context.Background(), []model.Message{{
		Role: model.RoleUser, Content: "inspect README",
	}}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := len(handler.calls), 2; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if got, want := result.Rounds, 5; got != want {
		t.Fatalf("rounds = %d, want %d", got, want)
	}
	if got := len(client.requests[4].Tools); got != 0 {
		t.Fatalf("synthesis round tools = %d, want 0", got)
	}
	last := client.requests[4].Messages[len(client.requests[4].Messages)-1]
	if last.Role != model.RoleSystem || last.Content != NoProgressPrompt {
		t.Fatalf("synthesis prompt = %#v, want NoProgressPrompt", last)
	}
}

func TestLoopIgnoresToolCallAfterSustainedNoProgressDetection(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: repeatedReadEvents("read-1")},
		{events: repeatedReadEvents("read-2")},
		{events: repeatedReadEvents("read-3")},
		{events: repeatedReadEvents("read-4")},
		{events: repeatedReadEvents("read-5")},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow)

	result, err := loop.Run(context.Background(), []model.Message{{
		Role: model.RoleUser, Content: "inspect README",
	}}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := len(handler.calls), 2; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if len(result.Message.ToolCalls) != 0 {
		t.Fatalf("final tool calls = %#v, want none", result.Message.ToolCalls)
	}
	if !strings.Contains(result.Message.Content, NoProgressFallback) {
		t.Fatalf("final content = %q, want no-progress fallback", result.Message.Content)
	}
}

func TestProgressGuardDefersSynthesisWhileExactCallIsSuppressed(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "read", Kind: tool.KindRead}}, 2)
	call := tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
	first := executedCall{call: call, result: tool.Result{CallID: call.ID, ToolName: call.Name, Output: "same"}}
	if stalled, err := guard.observeRound([]executedCall{first}); err != nil || stalled {
		t.Fatalf("first read stalled=%v err=%v", stalled, err)
	}
	call.ID = "r2"
	second := executedCall{call: call, result: tool.Result{CallID: call.ID, ToolName: call.Name, Output: "same"}}
	if stalled, err := guard.observeRound([]executedCall{second}); err != nil || stalled {
		t.Fatalf("second read escalated=%v err=%v, want recovery window", stalled, err)
	}
	for i := 3; i <= 4; i++ {
		call.ID = fmt.Sprintf("r%d", i)
		suppressed, err := guard.suppress(call)
		if err != nil || suppressed == nil {
			t.Fatalf("suppressed read %d = %#v err=%v", i, suppressed, err)
		}
		escalated, err := guard.observeRound([]executedCall{*suppressed})
		if err != nil {
			t.Fatalf("observe suppressed read %d: %v", i, err)
		}
		want := i == 4
		if escalated != want {
			t.Fatalf("suppressed read %d escalated=%v, want %v", i, escalated, want)
		}
	}
}

func TestProgressGuardDifferentEvidenceResetsSynthesisEscalation(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read", Kind: tool.KindRead},
		{Name: "grep", Kind: tool.KindGrep},
	}, 2)
	read := tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
	first := executedCall{call: read, result: tool.Result{CallID: read.ID, ToolName: read.Name, Output: "same"}}
	_, _ = guard.observeRound([]executedCall{first})
	read.ID = "r2"
	second := executedCall{call: read, result: tool.Result{CallID: read.ID, ToolName: read.Name, Output: "same"}}
	if escalated, err := guard.observeRound([]executedCall{second}); err != nil || escalated {
		t.Fatalf("second read escalated=%v err=%v", escalated, err)
	}
	grep := executedCall{
		call:   tool.Call{ID: "g1", Name: "grep", Arguments: json.RawMessage(`{"pattern":"TODO"}`)},
		result: tool.Result{CallID: "g1", ToolName: "grep", Output: "new evidence"},
	}
	if escalated, err := guard.observeRound([]executedCall{grep}); err != nil || escalated {
		t.Fatalf("different evidence escalated=%v err=%v", escalated, err)
	}
	read.ID = "r3"
	suppressed, err := guard.suppress(read)
	if err != nil || suppressed == nil {
		t.Fatalf("read after alternate evidence suppress=%#v err=%v", suppressed, err)
	}
	if escalated, err := guard.observeRound([]executedCall{*suppressed}); err != nil || escalated {
		t.Fatalf("stall after reset escalated=%v err=%v", escalated, err)
	}
}

func TestProgressGuardMutationResetsReadObservation(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read", Kind: tool.KindRead},
		{Name: "edit", Kind: tool.KindEdit},
	}, 2)
	read := executedCall{
		call:   tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)},
		result: tool.Result{CallID: "r1", ToolName: "read", Output: "same"},
	}

	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || stalled {
		t.Fatalf("first read stalled=%v err=%v", stalled, err)
	}
	read.call.ID = "r2"
	if stalled, tracked, err := guard.observe(read); err != nil || !tracked || !stalled {
		t.Fatalf("second identical read raw stalled=%v tracked=%v err=%v", stalled, tracked, err)
	}

	write := executedCall{
		call:   tool.Call{ID: "w1", Name: "edit", Arguments: json.RawMessage(`{"action":"write","path":"a.txt","content":"new"}`)},
		result: tool.Result{CallID: "w1", ToolName: "edit", Output: "written"},
	}
	if stalled, err := guard.observeRound([]executedCall{write}); err != nil || stalled {
		t.Fatalf("write stalled=%v err=%v", stalled, err)
	}

	read.call.ID = "r3"
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || stalled {
		t.Fatalf("read after mutation stalled=%v err=%v", stalled, err)
	}
}

func TestProgressGuardTaskMutationDoesNotResetRepositoryObservation(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
		{Name: "todo", Kind: tool.KindTask, Mutability: tool.MutabilityMutating, Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState}},
	}, 2)
	read := executedCall{
		call:   tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)},
		result: tool.Result{CallID: "r1", ToolName: "read", Output: "same"},
	}
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || stalled {
		t.Fatalf("first read stalled=%v err=%v", stalled, err)
	}
	taskUpdate := executedCall{
		call:   tool.Call{ID: "t1", Name: "todo", Arguments: json.RawMessage(`{"action":"update","expected_revision":0,"operations":[{"op":"add","id":"x","text":"x","status":"pending"}]}`)},
		result: tool.Result{CallID: "t1", ToolName: "todo", Output: "task plan revision 1"},
	}
	if stalled, err := guard.observeRound([]executedCall{taskUpdate}); err != nil || stalled {
		t.Fatalf("task update stalled=%v err=%v", stalled, err)
	}
	read.call.ID = "r2"
	if stalled, tracked, err := guard.observe(read); err != nil || !tracked || !stalled {
		t.Fatalf("second identical read after task metadata mutation raw stalled=%v tracked=%v err=%v", stalled, tracked, err)
	}
}

func TestProgressGuardTracksRepeatedTaskReads(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{
		Name: "todo", Kind: tool.KindTask, Mutability: tool.MutabilityReadOnly,
	}}, 2)
	call := tool.Call{ID: "t1", Name: "todo", Arguments: json.RawMessage(`{"action":"get"}`)}
	first := executedCall{call: call, result: tool.Result{CallID: "t1", ToolName: "todo", Output: "task snapshot revision 1"}}
	if stalled, err := guard.observeRound([]executedCall{first}); err != nil || stalled {
		t.Fatalf("first todo read stalled=%v err=%v", stalled, err)
	}
	call.ID = "t2"
	second := executedCall{call: call, result: tool.Result{CallID: "t2", ToolName: "todo", Output: "task snapshot revision 1"}}
	if stalled, tracked, err := guard.observe(second); err != nil || !tracked || !stalled {
		t.Fatalf("second identical todo read raw stalled=%v tracked=%v err=%v", stalled, tracked, err)
	}
}

func TestProgressGuardTracksRepeatedTaskMetadataMutation(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{
		Name: "todo", Kind: tool.KindTask, Mutability: tool.MutabilityMutating,
		Safety: tool.SafetyContract{MutationDomain: tool.MutationDomainTaskState},
	}}, 2)
	call := tool.Call{ID: "t1", Name: "todo", Arguments: json.RawMessage(`{"action":"update","expected_revision":1,"operations":[{"op":"set_status","id":"x","status":"in_progress"}]}`)}
	first := executedCall{call: call, result: tool.Result{CallID: "t1", ToolName: "todo", Output: "task plan revision 1 · no changes"}}
	if stalled, err := guard.observeRound([]executedCall{first}); err != nil || stalled {
		t.Fatalf("first todo update stalled=%v err=%v", stalled, err)
	}
	call.ID = "t2"
	second := executedCall{call: call, result: tool.Result{CallID: "t2", ToolName: "todo", Output: "task plan revision 1 · no changes"}}
	if stalled, tracked, err := guard.observe(second); err != nil || !tracked || !stalled {
		t.Fatalf("second identical todo update raw stalled=%v tracked=%v err=%v", stalled, tracked, err)
	}
}

func TestProgressGuardChangedResultIsProgress(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "read", Kind: tool.KindRead}}, 2)
	call := tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
	first := executedCall{call: call, result: tool.Result{CallID: "r1", ToolName: "read", Output: "v1"}}
	if stalled, err := guard.observeRound([]executedCall{first}); err != nil || stalled {
		t.Fatalf("first read stalled=%v err=%v", stalled, err)
	}
	call.ID = "r2"
	second := executedCall{call: call, result: tool.Result{CallID: "r2", ToolName: "read", Output: "v2"}}
	if stalled, err := guard.observeRound([]executedCall{second}); err != nil || stalled {
		t.Fatalf("changed result stalled=%v err=%v", stalled, err)
	}
}

func repeatedReadEvents(id string) []sdk.Event {
	return []sdk.Event{
		{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
			ID: id, Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`),
		}},
		{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
	}
}

func TestProgressGuardTracksRepeatedNonRetryableFailure(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "read", Kind: tool.KindRead}}, 2)
	call := tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"missing.txt"}`)}
	failure := &tool.Failure{Code: tool.ErrorCodeNotFound, Message: "missing"}
	first := executedCall{call: call, result: tool.Result{CallID: "r1", ToolName: "read", Failure: failure}}
	if stalled, err := guard.observeRound([]executedCall{first}); err != nil || stalled {
		t.Fatalf("first failure stalled=%v err=%v", stalled, err)
	}
	call.ID = "r2"
	second := executedCall{call: call, result: tool.Result{CallID: "r2", ToolName: "read", Failure: failure}}
	if stalled, tracked, err := guard.observe(second); err != nil || !tracked || !stalled {
		t.Fatalf("second identical failure raw stalled=%v tracked=%v err=%v", stalled, tracked, err)
	}
}

func TestProgressGuardSuppressesRepeatedMissingReadWithDiscoveryEvidence(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "read", Kind: tool.KindRead}}, 2)
	call := tool.Call{ID: "missing-1", Name: "read", Arguments: json.RawMessage(`{"path":"internal/base/runtimepolicy/runtimepolicy.go"}`)}
	failure := &tool.Failure{
		Code:     tool.ErrorCodeNotFound,
		Message:  `not found: "internal/base/runtimepolicy/runtimepolicy.go"`,
		Recovery: &tool.Recovery{Action: tool.RecoveryDiscoverResource, Tool: tool.NameLS, Arguments: json.RawMessage(`{"path":"internal/base/runtimepolicy"}`)},
		RecoveryEvidence: &tool.RecoveryEvidence{
			Action: tool.RecoveryDiscoverResource, Tool: tool.NameLS,
			StructuredOutput: json.RawMessage(`{"path":"internal/base/runtimepolicy","entries":[{"name":"defaults.go","kind":"file"}]}`),
		},
	}
	first := executedCall{call: call, result: tool.Result{CallID: call.ID, ToolName: call.Name, Failure: failure}, err: fmt.Errorf("missing")}
	if stalled, err := guard.observeRound([]executedCall{first}); err != nil || stalled {
		t.Fatalf("first missing read stalled=%v err=%v", stalled, err)
	}
	call.ID = "missing-2"
	suppressed, err := guard.suppress(call)
	if err != nil || suppressed == nil {
		t.Fatalf("second missing read suppress=%#v err=%v", suppressed, err)
	}
	if suppressed.suppressionReason != "terminal_failure" {
		t.Fatalf("suppression reason=%q, want terminal_failure", suppressed.suppressionReason)
	}
	if got := suppressed.result.Failure; got == nil || got.RecoveryEvidence == nil || len(got.RecoveryEvidence.StructuredOutput) == 0 {
		t.Fatalf("suppressed result lost discovery evidence: %#v", suppressed.result)
	}
}

func TestProgressGuardSuppressionPreservesActionableFailureRecovery(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "edit", Kind: tool.KindEdit}}, 2)
	call := tool.Call{ID: "edit-1", Name: "edit", Arguments: json.RawMessage(`{"file_path":"a.txt","content":"new"}`)}
	failure := &tool.Failure{
		Code:       tool.ErrorCodeInvalidArguments,
		Message:    "expected_sha256 is required when overwriting an existing file; call read first",
		Diagnostic: "read the current file before retrying the edit",
		Recovery: &tool.Recovery{
			Action: tool.RecoveryRefreshResource, Tool: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`),
		},
		RecoveryEvidence: &tool.RecoveryEvidence{
			Action: tool.RecoveryRefreshResource, Tool: "read", Output: "old", SHA256: "abc123",
		},
	}
	first := executedCall{call: call, result: tool.Result{CallID: call.ID, ToolName: call.Name, Failure: failure}, err: fmt.Errorf("edit failed")}
	if stalled, err := guard.observeRound([]executedCall{first}); err != nil || stalled {
		t.Fatalf("first failure stalled=%v err=%v", stalled, err)
	}
	call.ID = "edit-2"
	suppressed, err := guard.suppress(call)
	if err != nil || suppressed == nil {
		t.Fatalf("suppress = %#v err=%v", suppressed, err)
	}
	got := suppressed.result.Failure
	if got == nil || got.Code != failure.Code || got.Message != failure.Message || got.Diagnostic != failure.Diagnostic {
		t.Fatalf("suppressed failure = %#v, want original actionable failure", got)
	}
	if got.Recovery == nil || got.Recovery.Action != tool.RecoveryRefreshResource || string(got.Recovery.Arguments) != `{"path":"a.txt"}` {
		t.Fatalf("suppressed recovery = %#v", got.Recovery)
	}
	if got.RecoveryEvidence == nil || got.RecoveryEvidence.Output != "old" || got.RecoveryEvidence.SHA256 != "abc123" {
		t.Fatalf("suppressed recovery evidence = %#v", got.RecoveryEvidence)
	}
	if got == failure || got.Recovery == failure.Recovery || got.RecoveryEvidence == failure.RecoveryEvidence {
		t.Fatal("suppression reused mutable failure pointers")
	}
}

func TestNoProgressFailureHashIncludesRecoverySemantics(t *testing.T) {
	left, err := noProgressResultHash(tool.Result{Failure: &tool.Failure{
		Code:     tool.ErrorCodeInvalidArguments,
		Recovery: &tool.Recovery{Action: tool.RecoveryRefreshResource, Tool: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := noProgressResultHash(tool.Result{Failure: &tool.Failure{
		Code:     tool.ErrorCodeInvalidArguments,
		Recovery: &tool.Recovery{Action: tool.RecoveryRefreshResource, Tool: "read", Arguments: json.RawMessage(`{"path":"b.txt"}`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("failure hashes ignored distinct recovery arguments")
	}
}

func TestProgressGuardBoundsRetryableFailure(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "read", Kind: tool.KindRead}}, 2)
	for i := 1; i <= defaultMaxIdenticalRetryableFailures; i++ {
		call := tool.Call{ID: fmt.Sprintf("retry-%d", i), Name: "read", Arguments: json.RawMessage(`{"path":"slow.txt"}`)}
		failure := &tool.Failure{
			Code:      tool.ErrorCodeDeadlineExceeded,
			Message:   fmt.Sprintf("timeout attempt %d", i),
			Retryable: true,
		}
		execution := executedCall{call: call, result: tool.Result{CallID: call.ID, ToolName: call.Name, Failure: failure}}
		stalled, tracked, err := guard.observe(execution)
		if err != nil {
			t.Fatalf("retryable failure %d error = %v", i, err)
		}
		if !tracked {
			t.Fatalf("retryable failure %d was not tracked", i)
		}
		wantStalled := i == defaultMaxIdenticalRetryableFailures
		if stalled != wantStalled {
			t.Fatalf("retryable failure %d raw stalled=%v, want %v", i, stalled, wantStalled)
		}
	}
}

func TestLoopForcesSynthesisAfterSustainedRetryableFailureBudget(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: repeatedReadEvents("retry-1")},
		{events: repeatedReadEvents("retry-2")},
		{events: repeatedReadEvents("retry-3")},
		{events: repeatedReadEvents("retry-4")},
		{events: repeatedReadEvents("retry-5")},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "The repeated read timed out, so I stopped retrying."},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	handler := &retryableFailureHandler{definition: readFileDefinition()}
	loop := newLoopForHandler(
		t,
		client,
		handler,
		permission.ActionAllow,
		permission.ModeAlwaysApprove,
	)

	result, err := loop.Run(context.Background(), []model.Message{{
		Role: model.RoleUser, Content: "read the slow file",
	}}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := handler.calls, defaultMaxIdenticalRetryableFailures; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if got, want := result.Rounds, 6; got != want {
		t.Fatalf("rounds = %d, want %d", got, want)
	}
	if got := len(client.requests[len(client.requests)-1].Tools); got != 0 {
		t.Fatalf("synthesis request tools = %d, want 0", got)
	}
}

type retryableFailureHandler struct {
	definition tool.Definition
	calls      int
}

func (h *retryableFailureHandler) Definition() tool.Definition {
	return h.definition
}

func (h *retryableFailureHandler) Execute(context.Context, tool.Call) (tool.Result, error) {
	h.calls++
	return tool.Result{}, context.DeadlineExceeded
}

func TestSemanticCallHashCanonicalizesJSONObjectOrder(t *testing.T) {
	left, err := semanticCallHash(tool.Call{Name: "grep", Arguments: json.RawMessage(`{"path":".","pattern":"Loop"}`)})
	if err != nil {
		t.Fatalf("left hash error = %v", err)
	}
	right, err := semanticCallHash(tool.Call{Name: "grep", Arguments: json.RawMessage(`{"pattern":"Loop","path":"."}`)})
	if err != nil {
		t.Fatalf("right hash error = %v", err)
	}
	if left != right {
		t.Fatal("semantic hashes differ for equivalent JSON objects")
	}
}

func TestLoopSuppressesRepeatedPermissionPrompt(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: repeatedReadEvents("deny-1")},
		{events: repeatedReadEvents("deny-2")},
		{events: repeatedReadEvents("deny-3")},
		{events: repeatedReadEvents("deny-4")},
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "The read was denied."}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	handler := &recordingHandler{definition: readFileDefinition()}
	policy, err := permission.NewPolicy(permission.Config{Default: permission.ActionAsk})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	promptCalls := 0
	service, err := toolcall.NewService(
		&recordingRegistry{handler: handler},
		policy,
		toolcall.WithMode(permission.ModeAsk),
		toolcall.WithPrompt(func(context.Context, permission.Request) (permission.Resolution, error) {
			promptCalls++
			return permission.Resolution{Action: permission.ActionDeny}, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	loop, err := NewLoop(client, service)
	if err != nil {
		t.Fatalf("NewLoop() error = %v", err)
	}

	result, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "read README"}}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if promptCalls != 1 {
		t.Fatalf("permission prompts = %d, want 1", promptCalls)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("handler calls = %d, want 0", len(handler.calls))
	}
	if result.Rounds != 5 {
		t.Fatalf("rounds = %d, want 5", result.Rounds)
	}
	if len(client.requests[4].Tools) != 0 {
		t.Fatalf("synthesis tools = %d, want 0", len(client.requests[4].Tools))
	}
}

func TestProgressGuardSuppressesOnlyStalledCall(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read", Kind: tool.KindRead},
		{Name: "grep", Kind: tool.KindGrep},
	}, 2)
	dead := tool.Call{ID: "dead-1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
	for i := 0; i < 2; i++ {
		dead.ID = fmt.Sprintf("dead-%d", i+1)
		execution := executedCall{call: dead, result: tool.Result{CallID: dead.ID, ToolName: dead.Name, Output: "same"}}
		_, _, err := guard.observe(execution)
		if err != nil {
			t.Fatalf("observe dead call: %v", err)
		}
	}
	dead.ID = "dead-3"
	suppressed, err := guard.suppress(dead)
	if err != nil || suppressed == nil {
		t.Fatalf("dead suppress = %#v, err = %v", suppressed, err)
	}
	if suppressed.result.Failure == nil || suppressed.result.Failure.Code != tool.ErrorCodeNoProgress {
		t.Fatalf("suppressed failure = %#v", suppressed.result.Failure)
	}
	if suppressed.result.Failure.Diagnostic == "" {
		t.Fatal("suppressed no-progress result did not tell the model to change strategy")
	}
	live := tool.Call{ID: "live-1", Name: "grep", Arguments: json.RawMessage(`{"pattern":"TODO"}`)}
	allowed, err := guard.suppress(live)
	if err != nil {
		t.Fatalf("live suppress error = %v", err)
	}
	if allowed != nil {
		t.Fatalf("live call unexpectedly suppressed: %#v", allowed)
	}
}

func TestProgressGuardExplicitReadOnlyToolDoesNotResetEpoch(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
		{Name: "inspect_command", Kind: tool.KindBash, Mutability: tool.MutabilityReadOnly},
	}, 2)
	read := executedCall{
		call:   tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)},
		result: tool.Result{CallID: "r1", ToolName: "read", Output: "same"},
	}
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || stalled {
		t.Fatalf("first read stalled=%v err=%v", stalled, err)
	}
	inspect := executedCall{
		call:   tool.Call{ID: "i1", Name: "inspect_command", Arguments: json.RawMessage(`{"command":"pwd"}`)},
		result: tool.Result{CallID: "i1", ToolName: "inspect_command", Output: "/tmp"},
	}
	if stalled, err := guard.observeRound([]executedCall{inspect}); err != nil || stalled {
		t.Fatalf("inspect stalled=%v err=%v", stalled, err)
	}
	read.call.ID = "r2"
	if stalled, tracked, err := guard.observe(read); err != nil || !tracked || !stalled {
		t.Fatalf("second read raw stalled=%v tracked=%v err=%v", stalled, tracked, err)
	}
}

func TestProgressGuardReadOnlyBashDoesNotResetEpoch(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
		{Name: "bash", Kind: tool.KindBash, Mutability: tool.MutabilityMutating},
	}, 2)
	read := executedCall{call: tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)}, result: tool.Result{CallID: "r1", ToolName: "read", Output: "same"}}
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || stalled {
		t.Fatalf("first read stalled=%v err=%v", stalled, err)
	}
	inspect := executedCall{call: tool.Call{ID: "b1", Name: "bash", Arguments: json.RawMessage(`{"command":"git status --short"}`)}, result: tool.Result{CallID: "b1", ToolName: "bash", Output: ""}}
	if stalled, err := guard.observeRound([]executedCall{inspect}); err != nil || stalled {
		t.Fatalf("bash stalled=%v err=%v", stalled, err)
	}
	read.call.ID = "r2"
	if stalled, tracked, err := guard.observe(read); err != nil || !tracked || !stalled {
		t.Fatalf("second read raw stalled=%v tracked=%v err=%v", stalled, tracked, err)
	}
}

func TestProgressGuardMutatingBashResetsEpoch(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
		{Name: "bash", Kind: tool.KindBash, Mutability: tool.MutabilityMutating},
	}, 2)
	read := executedCall{call: tool.Call{ID: "r1", Name: "read", Arguments: json.RawMessage(`{"path":"a.txt"}`)}, result: tool.Result{CallID: "r1", ToolName: "read", Output: "same"}}
	_, _ = guard.observeRound([]executedCall{read})
	mutate := executedCall{call: tool.Call{ID: "b1", Name: "bash", Arguments: json.RawMessage(`{"command":"touch a.txt"}`)}, result: tool.Result{CallID: "b1", ToolName: "bash"}}
	if stalled, err := guard.observeRound([]executedCall{mutate}); err != nil || stalled {
		t.Fatalf("mutating bash stalled=%v err=%v", stalled, err)
	}
	read.call.ID = "r2"
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || stalled {
		t.Fatalf("read after mutation stalled=%v err=%v", stalled, err)
	}
}

type recordingProtectionObserver struct {
	events []toolcall.ProtectionEvent
}

func (o *recordingProtectionObserver) Observe(context.Context, toolcall.Event) {}
func (o *recordingProtectionObserver) ObserveProtection(_ context.Context, event toolcall.ProtectionEvent) {
	o.events = append(o.events, event)
}

func TestLoopEmitsPermissionRetrySuppressionTelemetry(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: repeatedReadEvents("deny-1")},
		{events: repeatedReadEvents("deny-2")},
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "permission remained denied"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	handler := &recordingHandler{definition: readFileDefinition()}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	observer := &recordingProtectionObserver{}
	service, err := toolcall.NewService(&recordingRegistry{handler: handler}, policy,
		toolcall.WithMode(permission.ModeAsk),
		toolcall.WithPrompt(func(context.Context, permission.Request) (permission.Resolution, error) {
			return permission.Resolution{Action: permission.ActionDeny, Reason: "test deny"}, nil
		}),
		toolcall.WithObserver(observer),
	)
	if err != nil {
		t.Fatal(err)
	}
	loop, err := NewLoop(client, service)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "read"}}, nil); err != nil {
		t.Fatal(err)
	}
	kinds := map[toolcall.ProtectionEventKind]toolcall.ProtectionEvent{}
	for _, event := range observer.events {
		kinds[event.Kind] = event
	}
	if _, ok := kinds[toolcall.ProtectionCallSuppressed]; !ok {
		t.Fatalf("events = %#v, missing call suppression", observer.events)
	}
	permissionEvent, ok := kinds[toolcall.ProtectionPermissionSuppressed]
	if !ok {
		t.Fatalf("events = %#v, missing permission suppression", observer.events)
	}
	if permissionEvent.Fingerprint == "" || permissionEvent.ToolName != "read" || permissionEvent.Reason != "permission_retry" {
		t.Fatalf("permission event = %#v", permissionEvent)
	}
}
