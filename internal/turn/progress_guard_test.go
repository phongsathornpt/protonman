package turn

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func TestLoopForcesSynthesisAfterRepeatedNoProgressRead(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID: "read-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`),
			}},
			{Kind: sdk.EventDone},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID: "read-2", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`),
			}},
			{Kind: sdk.EventDone},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "I already have the file contents."},
			{Kind: sdk.EventDone},
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
	if got, want := result.Rounds, 3; got != want {
		t.Fatalf("rounds = %d, want %d", got, want)
	}
	if got := len(client.requests[2].Tools); got != 0 {
		t.Fatalf("synthesis round tools = %d, want 0", got)
	}
	last := client.requests[2].Messages[len(client.requests[2].Messages)-1]
	if last.Role != model.RoleSystem || last.Content != NoProgressPrompt {
		t.Fatalf("synthesis prompt = %#v, want NoProgressPrompt", last)
	}
}

func TestLoopIgnoresToolCallAfterNoProgressDetection(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: repeatedReadEvents("read-1")},
		{events: repeatedReadEvents("read-2")},
		{events: repeatedReadEvents("read-3")},
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

func TestProgressGuardMutationResetsReadObservation(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read_file", Kind: tool.KindRead},
		{Name: "write_file", Kind: tool.KindEdit},
	}, 2)
	read := executedCall{
		call:   tool.Call{ID: "r1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)},
		result: tool.Result{CallID: "r1", ToolName: "read_file", Output: "same"},
	}

	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || stalled {
		t.Fatalf("first read stalled=%v err=%v", stalled, err)
	}
	read.call.ID = "r2"
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || !stalled {
		t.Fatalf("second identical read stalled=%v err=%v, want stalled", stalled, err)
	}

	write := executedCall{
		call:   tool.Call{ID: "w1", Name: "write_file", Arguments: json.RawMessage(`{"path":"a.txt","content":"new"}`)},
		result: tool.Result{CallID: "w1", ToolName: "write_file", Output: "written"},
	}
	if stalled, err := guard.observeRound([]executedCall{write}); err != nil || stalled {
		t.Fatalf("write stalled=%v err=%v", stalled, err)
	}

	read.call.ID = "r3"
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || stalled {
		t.Fatalf("read after mutation stalled=%v err=%v", stalled, err)
	}
}

func TestProgressGuardChangedResultIsProgress(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "read_file", Kind: tool.KindRead}}, 2)
	call := tool.Call{ID: "r1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
	first := executedCall{call: call, result: tool.Result{CallID: "r1", ToolName: "read_file", Output: "v1"}}
	if stalled, err := guard.observeRound([]executedCall{first}); err != nil || stalled {
		t.Fatalf("first read stalled=%v err=%v", stalled, err)
	}
	call.ID = "r2"
	second := executedCall{call: call, result: tool.Result{CallID: "r2", ToolName: "read_file", Output: "v2"}}
	if stalled, err := guard.observeRound([]executedCall{second}); err != nil || stalled {
		t.Fatalf("changed result stalled=%v err=%v", stalled, err)
	}
}

func repeatedReadEvents(id string) []sdk.Event {
	return []sdk.Event{
		{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
			ID: id, Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`),
		}},
		{Kind: sdk.EventDone},
	}
}

func TestProgressGuardTracksRepeatedNonRetryableFailure(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "read_file", Kind: tool.KindRead}}, 2)
	call := tool.Call{ID: "r1", Name: "read_file", Arguments: json.RawMessage(`{"path":"missing.txt"}`)}
	failure := &tool.Failure{Code: tool.ErrorCodeNotFound, Message: "missing"}
	first := executedCall{call: call, result: tool.Result{CallID: "r1", ToolName: "read_file", Failure: failure}}
	if stalled, err := guard.observeRound([]executedCall{first}); err != nil || stalled {
		t.Fatalf("first failure stalled=%v err=%v", stalled, err)
	}
	call.ID = "r2"
	second := executedCall{call: call, result: tool.Result{CallID: "r2", ToolName: "read_file", Failure: failure}}
	if stalled, err := guard.observeRound([]executedCall{second}); err != nil || !stalled {
		t.Fatalf("second identical failure stalled=%v err=%v, want stalled", stalled, err)
	}
}

func TestProgressGuardBoundsRetryableFailure(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{{Name: "read_file", Kind: tool.KindRead}}, 2)
	for i := 1; i <= defaultMaxIdenticalRetryableFailures; i++ {
		call := tool.Call{ID: fmt.Sprintf("retry-%d", i), Name: "read_file", Arguments: json.RawMessage(`{"path":"slow.txt"}`)}
		failure := &tool.Failure{
			Code:      tool.ErrorCodeDeadlineExceeded,
			Message:   fmt.Sprintf("timeout attempt %d", i),
			Retryable: true,
		}
		execution := executedCall{call: call, result: tool.Result{CallID: call.ID, ToolName: call.Name, Failure: failure}}
		stalled, err := guard.observeRound([]executedCall{execution})
		if err != nil {
			t.Fatalf("retryable failure %d error = %v", i, err)
		}
		wantStalled := i == defaultMaxIdenticalRetryableFailures
		if stalled != wantStalled {
			t.Fatalf("retryable failure %d stalled=%v, want %v", i, stalled, wantStalled)
		}
	}
}

func TestLoopForcesSynthesisAfterRetryableFailureBudget(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: repeatedReadEvents("retry-1")},
		{events: repeatedReadEvents("retry-2")},
		{events: repeatedReadEvents("retry-3")},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "The repeated read timed out, so I stopped retrying."},
			{Kind: sdk.EventDone},
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
	if got, want := result.Rounds, defaultMaxIdenticalRetryableFailures+1; got != want {
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
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "The read was denied."}, {Kind: sdk.EventDone}}},
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
	loop, err := NewLanguageModelLoop(client, service)
	if err != nil {
		t.Fatalf("NewLanguageModelLoop() error = %v", err)
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
	if result.Rounds != 3 {
		t.Fatalf("rounds = %d, want 3", result.Rounds)
	}
	if len(client.requests[2].Tools) != 0 {
		t.Fatalf("synthesis tools = %d, want 0", len(client.requests[2].Tools))
	}
}

func TestProgressGuardSuppressesOnlyStalledCall(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read_file", Kind: tool.KindRead},
		{Name: "grep", Kind: tool.KindGrep},
	}, 2)
	dead := tool.Call{ID: "dead-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)}
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
		{Name: "read_file", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
		{Name: "inspect_command", Kind: tool.KindBash, Mutability: tool.MutabilityReadOnly},
	}, 2)
	read := executedCall{
		call:   tool.Call{ID: "r1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)},
		result: tool.Result{CallID: "r1", ToolName: "read_file", Output: "same"},
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
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || !stalled {
		t.Fatalf("second read stalled=%v err=%v, want stalled without epoch reset", stalled, err)
	}
}

func TestProgressGuardReadOnlyBashDoesNotResetEpoch(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read_file", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
		{Name: "bash", Kind: tool.KindBash, Mutability: tool.MutabilityMutating},
	}, 2)
	read := executedCall{call: tool.Call{ID: "r1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)}, result: tool.Result{CallID: "r1", ToolName: "read_file", Output: "same"}}
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || stalled {
		t.Fatalf("first read stalled=%v err=%v", stalled, err)
	}
	inspect := executedCall{call: tool.Call{ID: "b1", Name: "bash", Arguments: json.RawMessage(`{"command":"git status --short"}`)}, result: tool.Result{CallID: "b1", ToolName: "bash", Output: ""}}
	if stalled, err := guard.observeRound([]executedCall{inspect}); err != nil || stalled {
		t.Fatalf("bash stalled=%v err=%v", stalled, err)
	}
	read.call.ID = "r2"
	if stalled, err := guard.observeRound([]executedCall{read}); err != nil || !stalled {
		t.Fatalf("second read stalled=%v err=%v, want stalled", stalled, err)
	}
}

func TestProgressGuardMutatingBashResetsEpoch(t *testing.T) {
	guard := newProgressGuard([]tool.Definition{
		{Name: "read_file", Kind: tool.KindRead, Mutability: tool.MutabilityReadOnly},
		{Name: "bash", Kind: tool.KindBash, Mutability: tool.MutabilityMutating},
	}, 2)
	read := executedCall{call: tool.Call{ID: "r1", Name: "read_file", Arguments: json.RawMessage(`{"path":"a.txt"}`)}, result: tool.Result{CallID: "r1", ToolName: "read_file", Output: "same"}}
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
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "permission remained denied"}, {Kind: sdk.EventDone}}},
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
	loop, err := NewLanguageModelLoop(client, service)
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
	if permissionEvent.Fingerprint == "" || permissionEvent.ToolName != "read_file" || permissionEvent.Reason != "permission_retry" {
		t.Fatalf("permission event = %#v", permissionEvent)
	}
}
