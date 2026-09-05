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
)

func TestLoopForcesSynthesisAfterRepeatedNoProgressRead(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []model.Event{
			{Kind: model.EventToolCall, ToolCall: model.ToolCall{
				ID: "read-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`),
			}},
			{Kind: model.EventDone},
		}},
		{events: []model.Event{
			{Kind: model.EventToolCall, ToolCall: model.ToolCall{
				ID: "read-2", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`),
			}},
			{Kind: model.EventDone},
		}},
		{events: []model.Event{
			{Kind: model.EventTextDelta, Text: "I already have the file contents."},
			{Kind: model.EventDone},
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

func repeatedReadEvents(id string) []model.Event {
	return []model.Event{
		{Kind: model.EventToolCall, ToolCall: model.ToolCall{
			ID: id, Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`),
		}},
		{Kind: model.EventDone},
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
		{events: []model.Event{
			{Kind: model.EventTextDelta, Text: "The repeated read timed out, so I stopped retrying."},
			{Kind: model.EventDone},
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
