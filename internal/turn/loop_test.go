package turn

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/projectTHORN/proton/internal/agentprompt"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/modelprofile"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func TestLoopBuildsEffectiveSystemPromptFromRuntime(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("follow project rules"), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &scriptedClient{streams: []scriptedStreamSpec{{events: []sdk.Event{
		{Kind: sdk.EventTextDelta, Text: "done"},
		{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
	}}}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithSystemPromptSpec(agentprompt.Spec{
		Role: "Inspect the assigned code carefully.", Profile: "reviewer", Workspace: workspace,
	}))
	_, err := loop.Run(context.Background(), []model.Message{
		{Role: model.RoleSystem, Content: "custom project instruction"},
		{Role: model.RoleUser, Content: "inspect"},
	}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(client.requests) != 1 || len(client.requests[0].Messages) < 2 {
		t.Fatalf("requests = %#v", client.requests)
	}
	system := client.requests[0].Messages[0]
	if system.Role != model.RoleSystem {
		t.Fatalf("first role = %q, want system", system.Role)
	}
	for _, want := range []string{
		`<proton-system-prompt version="2">`, "provider=test", "model=scripted", "profile=reviewer",
		"Workspace root: " + workspace, "Available tools: read_file.", "custom project instruction", "Inspect the assigned code carefully.", "follow project rules",
	} {
		if !strings.Contains(system.Content, want) {
			t.Fatalf("system prompt missing %q:\n%s", want, system.Content)
		}
	}
	if strings.Contains(system.Content, skillPromptMarker) {
		t.Fatalf("managed prompt used legacy skill marker: %s", system.Content)
	}
}

func TestLoopStreamsTextAndCompletes(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "hello"},
			{Kind: sdk.EventTextDelta, Text: " world"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		},
	}}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow)
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "say hello"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.Message.Content, "hello world"; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
	if got, want := result.Rounds, 1; got != want {
		t.Fatalf("rounds = %d, want %d", got, want)
	}
	if got, want := len(client.requests), 1; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	if got, want := len(client.requests[0].Tools), 1; got != want {
		t.Fatalf("published tools = %d, want %d", got, want)
	}
	gotKinds := eventKinds(events)
	wantKinds := []EventKind{EventTextDelta, EventTextDelta, EventCompleted}
	if !sameKinds(gotKinds, wantKinds) {
		t.Fatalf("events = %#v, want %#v", gotKinds, wantKinds)
	}
}

func TestLoopRejectsIncompleteModelStream(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "partial"}},
	}}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow)
	events := make([]Event, 0)

	_, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "hello"}},
		collectEvents(&events),
	)
	if !errors.Is(err, sdk.ErrIncompleteStream) {
		t.Fatalf("Run() error = %v, want incomplete stream", err)
	}
	if got := events[len(events)-1].Kind; got != EventFailed {
		t.Fatalf("last event kind = %q, want failed", got)
	}
}

func TestLoopRejectsEmptyModelResponse(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}},
	}}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow)

	_, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "hello"}},
		func(context.Context, Event) error { return nil },
	)
	if !errors.Is(err, ErrEmptyResponse) {
		t.Fatalf("Run() error = %v, want empty response", err)
	}
}

func TestLoopRejectsDuplicateToolCallIDs(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID:        "duplicate",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"a.txt"}`),
			}},
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID:        "duplicate",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"b.txt"}`),
			}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		},
	}}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow)

	_, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read both"}},
		func(context.Context, Event) error { return nil },
	)
	if !errors.Is(err, ErrDuplicateToolCall) {
		t.Fatalf("Run() error = %v, want duplicate tool call", err)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("handler calls = %d, want 0", len(handler.calls))
	}
}

func TestLoopCompletesWhenModelRequestsToolAtMaxRounds(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID:        "late-call",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"a.txt"}`),
			}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		},
	}}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithMaxRounds(1))
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read a file"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want graceful max-round fallback", err)
	}
	if len(result.Message.ToolCalls) != 0 {
		t.Fatalf("result tool calls = %#v, want none", result.Message.ToolCalls)
	}
	if !strings.Contains(result.Message.Content, MaxRoundsFallback) {
		t.Fatalf("result content = %q, want max-round fallback", result.Message.Content)
	}
	if got := events[len(events)-1].Kind; got != EventCompleted {
		t.Fatalf("last event kind = %q, want %q", got, EventCompleted)
	}
}

func TestLoopReportsToolCallWhenNoToolsAreAvailable(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID:        "unavailable-call",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"a.txt"}`),
			}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		},
	}}}
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(emptyRegistry{}, policy)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	loop, err := NewLoop(client, service)
	if err != nil {
		t.Fatalf("NewLoop() error = %v", err)
	}
	events := make([]Event, 0)

	_, err = loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read a file"}},
		collectEvents(&events),
	)
	if !errors.Is(err, ErrToolDispatchUnavailable) {
		t.Fatalf("Run() error = %v, want unavailable tool dispatch", err)
	}
	if errors.Is(err, ErrUnresolvedToolCall) {
		t.Fatalf("Run() error = %v, must not be classified as unresolved dispatch", err)
	}
	if got := events[len(events)-1].Kind; got != EventFailed {
		t.Fatalf("last event kind = %q, want %q", got, EventFailed)
	}
	if got := len(client.requests[0].Tools); got != 0 {
		t.Fatalf("published tools = %d, want none", got)
	}
}

func TestLoopStopsWhenToolCallBatchExceedsCumulativeLimit(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID:        "over-budget-1",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"a.txt"}`),
			}},
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID:        "over-budget-2",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"b.txt"}`),
			}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		},
	}}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow, WithMaxToolCalls(1))

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read both files"}},
		func(context.Context, Event) error { return nil },
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want graceful max-tool-call fallback", err)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("handler calls = %d, want 0 when batch exceeds budget", len(handler.calls))
	}
	if len(result.Message.ToolCalls) != 0 {
		t.Fatalf("result tool calls = %#v, want none", result.Message.ToolCalls)
	}
	if !strings.Contains(result.Message.Content, MaxToolCallsFallback) {
		t.Fatalf("result content = %q, want max-tool-call fallback", result.Message.Content)
	}
}

func TestLoopAppliesCumulativeToolCallLimitAcrossRounds(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID:        "budgeted-call",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"a.txt"}`),
			}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "The budgeted read completed."},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow, WithMaxToolCalls(1))

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read a file"}},
		func(context.Context, Event) error { return nil },
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := len(handler.calls), 1; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if got, want := result.Rounds, 2; got != want {
		t.Fatalf("rounds = %d, want %d", got, want)
	}
	if got, want := len(client.requests[1].Tools), 0; got != want {
		t.Fatalf("second request published tools = %d, want %d", got, want)
	}
	lastMessage := client.requests[1].Messages[len(client.requests[1].Messages)-1]
	if lastMessage.Role != model.RoleSystem || lastMessage.Content != MaxToolCallsPrompt {
		t.Fatalf("second request last message = %#v, want max-tool-call prompt", lastMessage)
	}
	if got, want := result.Message.Content, "The budgeted read completed."; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
}

func TestLoopPreservesTextWhenMaxRoundToolCallIsIgnored(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "partial answer"},
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{
				ID:        "late-call",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"a.txt"}`),
			}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		},
	}}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow, WithMaxRounds(1))

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read a file"}},
		func(context.Context, Event) error { return nil },
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want graceful max-round fallback", err)
	}
	if !strings.Contains(result.Message.Content, "partial answer") || !strings.Contains(result.Message.Content, MaxRoundsFallback) {
		t.Fatalf("result content = %q, want preserved text and fallback", result.Message.Content)
	}
	if len(result.Message.ToolCalls) != 0 {
		t.Fatalf("result tool calls = %#v, want none", result.Message.ToolCalls)
	}
	if len(handler.calls) != 0 {
		t.Fatalf("handler calls = %d, want 0", len(handler.calls))
	}
}

func TestLoopTranslatesToolCallsAndFeedsResultsBack(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{
				Kind: sdk.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-1",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"README.md"}`),
				},
			},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "I found it."},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow)
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read the README"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.Message.Content, "I found it."; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
	if got, want := len(handler.calls), 1; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if got, want := len(result.Messages), 3; got != want {
		t.Fatalf("turn messages = %d, want %d", got, want)
	}
	if result.Messages[0].Role != model.RoleAssistant || len(result.Messages[0].ToolCalls) != 1 {
		t.Fatalf("first turn message = %#v, want assistant tool call", result.Messages[0])
	}
	if result.Messages[1].Role != model.RoleTool || result.Messages[1].ToolCallID != "call-1" {
		t.Fatalf("second turn message = %#v, want matching tool result", result.Messages[1])
	}
	if result.Messages[2].Role != model.RoleAssistant || result.Messages[2].Content != "I found it." {
		t.Fatalf("final turn message = %#v, want final assistant", result.Messages[2])
	}
	if got, want := len(client.requests), 2; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	followUp := client.requests[1].Messages
	if got, want := len(followUp), 3; got != want {
		t.Fatalf("follow-up messages = %d, want %d", got, want)
	}
	if got, want := len(followUp[1].ToolCalls), 1; got != want {
		t.Fatalf("assistant tool calls = %d, want %d", got, want)
	}
	if got, want := followUp[1].ToolCalls[0].ID, "call-1"; got != want {
		t.Fatalf("assistant tool call ID = %q, want %q", got, want)
	}
	var toolMessage tool.Result
	if err := json.Unmarshal([]byte(followUp[2].Content), &toolMessage); err != nil {
		t.Fatalf("decode follow-up tool result: %v", err)
	}
	if got, want := toolMessage.Output, "file contents"; got != want {
		t.Fatalf("follow-up tool output = %q, want %q", got, want)
	}
	gotKinds := eventKinds(events)
	wantKinds := []EventKind{EventToolCall, EventToolResult, EventTextDelta, EventCompleted}
	if !sameKinds(gotKinds, wantKinds) {
		t.Fatalf("events = %#v, want %#v", gotKinds, wantKinds)
	}
}

func TestLoopKeepsPermissionDenialInsideToolConversation(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{
				Kind: sdk.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-denied",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":".env"}`),
				},
			},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "I cannot access that file."},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionDeny)
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read the secret"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := len(handler.calls), 0; got != want {
		t.Fatalf("handler calls = %d, want %d", got, want)
	}
	if got, want := result.Message.Content, "I cannot access that file."; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
	toolEvent := findEvent(events, EventToolResult)
	if toolEvent.Err == nil {
		t.Fatal("tool result error = nil, want permission denial")
	}
	if toolEvent.Result.Failure == nil || toolEvent.Result.Failure.Code != tool.ErrorCodePermissionDenied {
		t.Fatalf("tool result failure = %#v, want permission_denied", toolEvent.Result.Failure)
	}
	if got := client.requests[1].Messages[2].Content; !contains(got, string(tool.ErrorCodePermissionDenied)) {
		t.Fatalf("follow-up denial content = %q, want permission code", got)
	}
	if !client.requests[1].Messages[2].ToolResultIsError {
		t.Fatal("follow-up denial tool message should be marked as an error")
	}
}

func TestLoopGracefulMaxRoundsSynthesis(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{
			events: []sdk.Event{
				{
					Kind: sdk.EventToolCall,
					ToolCall: model.ToolCall{
						ID:        "call-1",
						Name:      "read_file",
						Arguments: json.RawMessage(`{"path":"README.md"}`),
					},
				},
				{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
			},
		},
		{
			events: []sdk.Event{
				{Kind: sdk.EventTextDelta, Text: "Reached max rounds. Accomplished: read README. Remaining: none."},
				{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
			},
		},
	}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithMaxRounds(2))
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "keep going"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if got, want := result.Rounds, 2; got != want {
		t.Fatalf("result.Rounds = %d, want %d", got, want)
	}
	if got, want := len(client.requests), 2; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	// First round had tools available.
	if got, want := len(client.requests[0].Tools), 1; got != want {
		t.Fatalf("round 1 tools = %d, want %d", got, want)
	}
	// Second round reached maxRounds: tools stripped, MaxRoundsPrompt injected.
	if got, want := len(client.requests[1].Tools), 0; got != want {
		t.Fatalf("round 2 tools = %d, want %d", got, want)
	}
	round2Msgs := client.requests[1].Messages
	lastMsg := round2Msgs[len(round2Msgs)-1]
	if lastMsg.Role != model.RoleSystem || lastMsg.Content != MaxRoundsPrompt {
		t.Fatalf("round 2 last message = %#v, want system MaxRoundsPrompt", lastMsg)
	}
	if !strings.Contains(result.Message.Content, "Accomplished: read README") {
		t.Fatalf("result content = %q, want summary text", result.Message.Content)
	}
	if got := events[len(events)-1].Kind; got != EventCompleted {
		t.Fatalf("last event kind = %q, want %q", got, EventCompleted)
	}
}

func TestLoopStopsAtMaxRoundsWhenOne(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "Single round synthesis: all set."},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		},
	}}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithMaxRounds(1))
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "quick answer"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if got, want := result.Rounds, 1; got != want {
		t.Fatalf("rounds = %d, want %d", got, want)
	}
	if got, want := len(client.requests[0].Tools), 0; got != want {
		t.Fatalf("round 1 tools = %d, want %d", got, want)
	}
	reqMsgs := client.requests[0].Messages
	lastMsg := reqMsgs[len(reqMsgs)-1]
	if lastMsg.Role != model.RoleSystem || lastMsg.Content != MaxRoundsPrompt {
		t.Fatalf("round 1 last message = %#v, want system MaxRoundsPrompt", lastMsg)
	}
	if got := events[len(events)-1].Kind; got != EventCompleted {
		t.Fatalf("last event kind = %q, want %q", got, EventCompleted)
	}
}

func TestLoopUnboundedWhenZero(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{
			events: []sdk.Event{
				{
					Kind: sdk.EventToolCall,
					ToolCall: model.ToolCall{
						ID:        "call-1",
						Name:      "read_file",
						Arguments: json.RawMessage(`{"path":"a.txt"}`),
					},
				},
				{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
			},
		},
		{
			events: []sdk.Event{
				{
					Kind: sdk.EventToolCall,
					ToolCall: model.ToolCall{
						ID:        "call-2",
						Name:      "read_file",
						Arguments: json.RawMessage(`{"path":"b.txt"}`),
					},
				},
				{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
			},
		},
		{
			events: []sdk.Event{
				{
					Kind: sdk.EventToolCall,
					ToolCall: model.ToolCall{
						ID:        "call-3",
						Name:      "read_file",
						Arguments: json.RawMessage(`{"path":"c.txt"}`),
					},
				},
				{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
			},
		},
		{
			events: []sdk.Event{
				{Kind: sdk.EventTextDelta, Text: "Processed all 3 files unbounded."},
				{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
			},
		},
	}}
	// WithMaxRounds(0) indicates unbounded.
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithMaxRounds(0))
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "process files"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.Rounds, 4; got != want {
		t.Fatalf("result.Rounds = %d, want %d", got, want)
	}
	// Verify tools were provided on round 1, 2, 3 and round 4
	for i := 0; i < 4; i++ {
		if got, want := len(client.requests[i].Tools), 1; got != want {
			t.Fatalf("round %d tools = %d, want %d", i+1, got, want)
		}
	}
	if got := events[len(events)-1].Kind; got != EventCompleted {
		t.Fatalf("last event kind = %q, want %q", got, EventCompleted)
	}
}

func TestLoopTimesOutIndividualToolCall(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{
				Kind: sdk.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-timeout",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"slow.txt"}`),
				},
			},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "timed out safely"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	handler := &contextBlockingHandler{
		definition: readFileDefinition(),
		started:    make(chan struct{}),
	}
	loop := newLoopForHandler(
		t,
		client,
		handler,
		permission.ActionAllow,
		permission.ModeAsk,
		WithToolTimeout(20*time.Millisecond),
	)
	events := make([]Event, 0)
	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "read slowly"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := result.Message.Content, "timed out safely"; got != want {
		t.Fatalf("final content = %q, want %q", got, want)
	}
	toolEvent := findEvent(events, EventToolResult)
	if toolEvent.Result.Failure == nil || toolEvent.Result.Failure.Code != tool.ErrorCodeDeadlineExceeded {
		t.Fatalf("tool result failure = %#v, want deadline_exceeded", toolEvent.Result.Failure)
	}
}

func TestLoopCancelsModelStreamWithParentContext(t *testing.T) {
	client := &blockingModelClient{started: make(chan struct{})}
	loop, _ := newTestLoop(t, client, permission.ActionAllow)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make([]Event, 0)
	resultCh := make(chan error, 1)
	go func() {
		_, err := loop.Run(
			ctx,
			[]model.Message{{Role: model.RoleUser, Content: "wait"}},
			collectEvents(&events),
		)
		resultCh <- err
	}()
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("model stream did not start")
	}
	cancel()
	select {
	case err := <-resultCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}
	if got := events[len(events)-1].Kind; got != EventFailed {
		t.Fatalf("last event kind = %q, want %q", got, EventFailed)
	}
}

func TestLoopRunsApprovedReadCallsWithBoundedConcurrency(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{
				Kind: sdk.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-read-1",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"one.txt"}`),
				},
			},
			{
				Kind: sdk.EventToolCall,
				ToolCall: model.ToolCall{
					ID:        "call-read-2",
					Name:      "read_file",
					Arguments: json.RawMessage(`{"path":"two.txt"}`),
				},
			},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "both read"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	handler := &parallelHandler{
		definition: readFileDefinition(),
		started:    make(chan struct{}, 2),
		release:    make(chan struct{}),
	}
	loop := newLoopForHandler(
		t,
		client,
		handler,
		permission.ActionAllow,
		permission.ModeAlwaysApprove,
		WithMaxParallelReads(2),
	)
	resultCh := make(chan struct {
		result Result
		err    error
	}, 1)
	go func() {
		result, err := loop.Run(
			context.Background(),
			[]model.Message{{Role: model.RoleUser, Content: "read both"}},
			nil,
		)
		resultCh <- struct {
			result Result
			err    error
		}{result: result, err: err}
	}()
	for range 2 {
		select {
		case <-handler.started:
		case <-time.After(time.Second):
			t.Fatal("read calls did not start concurrently")
		}
	}
	close(handler.release)
	select {
	case outcome := <-resultCh:
		if outcome.err != nil {
			t.Fatalf("Run() error = %v", outcome.err)
		}
		if got, want := outcome.result.Message.Content, "both read"; got != want {
			t.Fatalf("final content = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not complete after read calls were released")
	}
	if got, want := handler.max.Load(), int32(2); got != want {
		t.Fatalf("maximum concurrent calls = %d, want %d", got, want)
	}
}

type recordingHandler struct {
	definition       tool.Definition
	calls            []tool.Call
	output           string
	structuredOutput json.RawMessage
}

func readFileDefinition() tool.Definition {
	return tool.Definition{
		Name:                "read_file",
		Description:         "read a file",
		Kind:                tool.KindRead,
		Evidence:            tool.EvidenceWorkspace,
		PermissionDetailKey: "path",
	}
}

type contextBlockingHandler struct {
	definition tool.Definition
	started    chan struct{}
	once       sync.Once
}

func (h *contextBlockingHandler) Definition() tool.Definition {
	return h.definition
}

func (h *contextBlockingHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	h.once.Do(func() { close(h.started) })
	<-ctx.Done()
	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
	}, ctx.Err()
}

type parallelHandler struct {
	definition tool.Definition
	started    chan struct{}
	release    chan struct{}
	current    atomic.Int32
	max        atomic.Int32
}

func (h *parallelHandler) Definition() tool.Definition {
	return h.definition
}

func (h *parallelHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	current := h.current.Add(1)
	for {
		maximum := h.max.Load()
		if current <= maximum || h.max.CompareAndSwap(maximum, current) {
			break
		}
	}
	h.started <- struct{}{}
	select {
	case <-h.release:
	case <-ctx.Done():
	}
	h.current.Add(-1)
	return tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   "file contents",
	}, nil
}

func (h *recordingHandler) Definition() tool.Definition {
	return h.definition
}

func (h *recordingHandler) Execute(_ context.Context, call tool.Call) (tool.Result, error) {
	h.calls = append(h.calls, call)
	output := h.output
	if output == "" {
		output = "file contents"
	}
	return tool.Result{
		CallID:           call.ID,
		ToolName:         call.Name,
		Output:           output,
		StructuredOutput: append(json.RawMessage(nil), h.structuredOutput...),
	}, nil
}

type recordingRegistry struct {
	handler tool.Handler
}

type emptyRegistry struct{}

func (emptyRegistry) Lookup(string) (tool.Handler, bool) {
	return nil, false
}

func (emptyRegistry) Definitions() []tool.Definition {
	return []tool.Definition{}
}

func (r *recordingRegistry) Lookup(name string) (tool.Handler, bool) {
	if name != r.handler.Definition().Name {
		return nil, false
	}
	return r.handler, true
}

func (r *recordingRegistry) Definitions() []tool.Definition {
	return []tool.Definition{r.handler.Definition()}
}

func TestLoopAppliesReasoningEffortFromKnownModelProfile(t *testing.T) {
	client := &scriptedClient{
		profile: modelprofile.ResolveBuiltin("gateway", "gemini-3.8-flash", modelprofile.CatalogMetadata{}),
		streams: []scriptedStreamSpec{{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "done"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}}},
	}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithReasoningEffort(sdk.ReasoningHigh))
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := client.requests[0].Options.ReasoningEffort; got != sdk.ReasoningHigh {
		t.Fatalf("ReasoningEffort = %q, want high", got)
	}
}

func TestLoopClampsPortableReasoningEffortToKnownLevels(t *testing.T) {
	client := &scriptedClient{
		profile: modelprofile.Resolved{Reasoning: modelprofile.Reasoning{Support: modelprofile.SupportYes, Levels: []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium}}},
		streams: []scriptedStreamSpec{{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "done"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}}},
	}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithReasoningEffort(sdk.ReasoningHigh))
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := client.requests[0].Options.ReasoningEffort; got != sdk.ReasoningMedium {
		t.Fatalf("ReasoningEffort = %q, want medium", got)
	}
}

func TestLoopOmitsReasoningForUnknownModelProfile(t *testing.T) {
	client := &scriptedClient{
		streams: []scriptedStreamSpec{{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "done"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}}},
	}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithReasoningEffort(sdk.ReasoningHigh))
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := client.requests[0].Options.ReasoningEffort; got != sdk.ReasoningDefault {
		t.Fatalf("ReasoningEffort = %q, want provider default", got)
	}
}

type scriptedStreamSpec struct {
	events   []sdk.Event
	closeErr error
}

type scriptedClient struct {
	streams  []scriptedStreamSpec
	requests []sdk.Request
	profile  modelprofile.Resolved
}

func (*scriptedClient) Provider() string                              { return "test" }
func (*scriptedClient) ModelID() string                               { return "scripted" }
func (c *scriptedClient) ResolvedModelProfile() modelprofile.Resolved { return c.profile }
func (*scriptedClient) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true}
}

func (c *scriptedClient) Stream(_ context.Context, request sdk.Request) (sdk.Stream, error) {
	if len(c.streams) == 0 {
		return nil, errors.New("no scripted model stream remains")
	}
	c.requests = append(c.requests, sdk.Request{
		Messages: sdk.CloneMessages(request.Messages),
		Tools:    append([]sdk.Tool(nil), request.Tools...),
		Options:  request.Options,
	})
	spec := c.streams[0]
	c.streams = c.streams[1:]
	return &scriptedStream{events: spec.events, closeErr: spec.closeErr}, nil
}

type scriptedStream struct {
	events   []sdk.Event
	index    int
	closeErr error
}

func (s *scriptedStream) Next(ctx context.Context) (sdk.Event, error) {
	if err := ctx.Err(); err != nil {
		return sdk.Event{}, err
	}
	if s.index >= len(s.events) {
		return sdk.Event{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}

func (s *scriptedStream) Close() error {
	return s.closeErr
}

func newTestLoop(
	t *testing.T,
	client sdk.LanguageModel,
	action permission.Action,
	options ...Option,
) (*Loop, *recordingHandler) {
	t.Helper()
	handler := &recordingHandler{definition: readFileDefinition()}
	loop := newLoopForHandler(
		t,
		client,
		handler,
		action,
		permission.ModeAsk,
		options...,
	)
	return loop, handler
}

func newLoopForHandler(
	t *testing.T,
	client sdk.LanguageModel,
	handler tool.Handler,
	action permission.Action,
	mode permission.Mode,
	options ...Option,
) *Loop {
	t.Helper()
	policy, err := permission.NewPolicy(permission.Config{
		Rules: []permission.Rule{{
			Action: action,
			Tool:   permission.ToolRead,
		}},
	})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(
		&recordingRegistry{handler: handler},
		policy,
		toolcall.WithMode(mode),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	loop, err := NewLoop(client, service, options...)
	if err != nil {
		t.Fatalf("NewLoop() error = %v", err)
	}
	return loop
}

type blockingModelClient struct {
	started chan struct{}
	once    sync.Once
}

func (*blockingModelClient) Provider() string { return "test" }
func (*blockingModelClient) ModelID() string  { return "blocking" }
func (*blockingModelClient) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}

func (c *blockingModelClient) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	c.once.Do(func() { close(c.started) })
	return blockingModelStream{}, nil
}

type blockingModelStream struct{}

func (blockingModelStream) Next(ctx context.Context) (sdk.Event, error) {
	<-ctx.Done()
	return sdk.Event{}, ctx.Err()
}

func (blockingModelStream) Close() error {
	return nil
}

func collectEvents(events *[]Event) Sink {
	return func(_ context.Context, event Event) error {
		*events = append(*events, event)
		return nil
	}
}

func eventKinds(events []Event) []EventKind {
	kinds := make([]EventKind, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func sameKinds(left []EventKind, right []EventKind) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func findEvent(events []Event, kind EventKind) Event {
	for _, event := range events {
		if event.Kind == kind {
			return event
		}
	}
	return Event{}
}

func contains(value string, target string) bool {
	return strings.Contains(value, target)
}

func TestLoopAugmentsSystemPromptWithSkillCatalog(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "I see the skills"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		},
	}}}
	catalog := []skill.CatalogItem{
		{
			Name:        "pdf-processing",
			Description: "Handle PDFs",
			Location:    "/path/to/SKILL.md",
			Scope:       skill.ScopeUser,
		},
	}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithSkillCatalog(catalog))
	events := make([]Event, 0)

	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "help with pdf"}},
		collectEvents(&events),
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Message.Content != "I see the skills" {
		t.Fatalf("unexpected content: %q", result.Message.Content)
	}

	if len(client.requests) == 0 {
		t.Fatalf("expected at least 1 model request")
	}
	reqMessages := client.requests[0].Messages
	if len(reqMessages) < 2 {
		t.Fatalf("expected at least 2 messages (system + user), got %d", len(reqMessages))
	}
	if reqMessages[0].Role != model.RoleSystem {
		t.Errorf("expected first message to be RoleSystem, got %s", reqMessages[0].Role)
	}
	if !strings.Contains(reqMessages[0].Content, "<available_skills>") || !strings.Contains(reqMessages[0].Content, "pdf-processing") {
		t.Errorf("system message missing skill catalog: %s", reqMessages[0].Content)
	}
}

func TestLoopDoesNotDuplicateSkillCatalogMarker(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "ok"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		},
	}}}
	catalog := []skill.CatalogItem{{
		Name:        "pdf-processing",
		Description: "Handle PDFs",
		Location:    "/path/to/SKILL.md",
		Scope:       skill.ScopeUser,
	}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithSkillCatalog(catalog))
	priorSystem := skillPromptMarker + "\n" + skill.SystemPromptSection(catalog)
	_, err := loop.Run(context.Background(), []model.Message{
		{Role: model.RoleSystem, Content: priorSystem},
		{Role: model.RoleUser, Content: "help with pdf"},
	}, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	content := client.requests[0].Messages[0].Content
	if got := strings.Count(content, skillPromptMarker); got != 1 {
		t.Fatalf("skill marker count = %d, want 1: %q", got, content)
	}
	if got := strings.Count(content, "<available_skills>"); got != 1 {
		t.Fatalf("skill catalog count = %d, want 1: %q", got, content)
	}
}

func TestLoopDynamicActiveSkillsWithRegistry(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "ok"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "ok"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}

	s1 := skill.Skill{
		Name:         "active-skill",
		Description:  "An active skill",
		Location:     "/loc/active/SKILL.md",
		BaseDir:      "/loc/active",
		Scope:        skill.ScopeUser,
		Instructions: "# Active Instructions\nDo active stuff.",
	}
	s2 := skill.Skill{
		Name:         "available-skill",
		Description:  "An available skill",
		Location:     "/loc/avail/SKILL.md",
		BaseDir:      "/loc/avail",
		Scope:        skill.ScopeProject,
		Instructions: "# Avail Instructions",
	}

	reg := skill.NewRegistry(s1, s2)
	reg.MarkActivated("active-skill")

	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithSkillRegistry(reg))

	// Turn 1: active-skill is active, available-skill is available
	_, err := loop.Run(context.Background(), []model.Message{
		{Role: model.RoleUser, Content: "turn 1"},
	}, nil)
	if err != nil {
		t.Fatalf("Run turn 1 error = %v", err)
	}

	req1 := client.requests[0]
	sys1 := req1.Messages[0].Content
	if !strings.Contains(sys1, "<active_skills>") || !strings.Contains(sys1, "Do active stuff") {
		t.Fatalf("expected active skill instructions in sys1, got: %s", sys1)
	}
	if !strings.Contains(sys1, "<available_skills>") || !strings.Contains(sys1, "available-skill") {
		t.Fatalf("expected available-skill in catalog in sys1, got: %s", sys1)
	}
	if strings.Contains(sys1, "<available_skills>") && strings.Contains(sys1, "<name>active-skill</name>") {
		t.Fatalf("active-skill should not be listed as available in sys1")
	}

	// Turn 2: deactivate active-skill -> moves to available
	reg.Deactivate("active-skill")
	_, err = loop.Run(context.Background(), []model.Message{
		{Role: model.RoleUser, Content: "turn 2"},
	}, nil)
	if err != nil {
		t.Fatalf("Run turn 2 error = %v", err)
	}

	req2 := client.requests[1]
	sys2 := req2.Messages[0].Content
	if strings.Contains(sys2, "<active_skills>") {
		t.Fatalf("expected no active_skills in sys2 after deactivation, got: %s", sys2)
	}
	if !strings.Contains(sys2, "<name>active-skill</name>") {
		t.Fatalf("expected active-skill back in available catalog in sys2, got: %s", sys2)
	}
}

func TestNewLoopRejectsAllGlobalBoundsDisabled(t *testing.T) {
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatalf("NewPolicy() error = %v", err)
	}
	service, err := toolcall.NewService(emptyRegistry{}, policy)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "done"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}},
	}}}
	_, err = NewLoop(
		client,
		service,
		WithMaxRounds(0),
		WithMaxToolCalls(0),
		WithTurnTimeout(0),
	)
	if !errors.Is(err, ErrInvalidLoop) {
		t.Fatalf("NewLoop() error = %v, want ErrInvalidLoop", err)
	}
}

func TestLoopWholeTurnTimeoutStopsBlockingModel(t *testing.T) {
	client := &blockingModelClient{started: make(chan struct{})}
	loop, _ := newTestLoop(
		t,
		client,
		permission.ActionAllow,
		WithTurnTimeout(20*time.Millisecond),
		WithRoundTimeout(0),
	)
	events := make([]Event, 0)

	_, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "wait forever"}},
		collectEvents(&events),
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error = %v, want context.DeadlineExceeded", err)
	}
	if len(events) == 0 || events[len(events)-1].Kind != EventFailed {
		t.Fatalf("events = %#v, want terminal failed event", eventKinds(events))
	}
}

func TestLoopAllowsUnboundedCountsWithFiniteTurnTimeout(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{{
		events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "bounded by time"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}},
	}}}
	loop, _ := newTestLoop(
		t,
		client,
		permission.ActionAllow,
		WithMaxRounds(0),
		WithMaxToolCalls(0),
		WithTurnTimeout(time.Second),
	)
	result, err := loop.Run(
		context.Background(),
		[]model.Message{{Role: model.RoleUser, Content: "answer"}},
		nil,
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Message.Content != "bounded by time" {
		t.Fatalf("content = %q", result.Message.Content)
	}
}

func TestFailUsesBoundedDetachedContextForTerminalEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	loop := &Loop{}
	var sawEvent bool
	_, err := loop.fail(ctx, func(eventCtx context.Context, event Event) error {
		sawEvent = true
		if event.Kind != EventFailed {
			t.Fatalf("event kind = %v, want EventFailed", event.Kind)
		}
		if eventCtx.Err() != nil {
			t.Fatalf("terminal event inherited cancellation: %v", eventCtx.Err())
		}
		deadline, ok := eventCtx.Deadline()
		if !ok {
			t.Fatal("terminal event context has no deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > terminalEmitTimeout {
			t.Fatalf("terminal event deadline remaining = %v", remaining)
		}
		return nil
	}, 1, context.Canceled)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("fail error = %v, want context canceled", err)
	}
	if !sawEvent {
		t.Fatal("terminal event was not emitted")
	}
}

func TestLoopPreservesStructuredToolOutputAsNestedJSON(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{ID: "structured", Name: "read_file", Arguments: json.RawMessage(`{"path":"TODO.md"}`)}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishToolCalls},
		}},
		{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "done"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow)
	handler.output = "task snapshot revision 4 · 1 tasks"
	handler.structuredOutput = json.RawMessage(`{"revision":4,"items":[{"id":"a"}]}`)
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect tasks"}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
	content := client.requests[1].Messages[len(client.requests[1].Messages)-1].Content
	var envelope map[string]any
	if err := json.Unmarshal([]byte(content), &envelope); err != nil {
		t.Fatalf("tool history is not JSON: %v (%s)", err, content)
	}
	structured, ok := envelope["structured_output"].(map[string]any)
	if !ok || structured["revision"] != float64(4) {
		t.Fatalf("structured_output = %#v, want nested JSON object", envelope["structured_output"])
	}
}

func TestLoopAppliesAggregateToolResultBudgetBeforeHistory(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{ID: "read-big", Name: "read_file", Arguments: json.RawMessage(`{"path":"big.txt"}`)}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishToolCalls},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "done"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	loop, handler := newTestLoop(t, client, permission.ActionAllow,
		WithMaxToolResultBytesPerRound(64), WithMaxToolResultBytesPerTurn(64))
	handler.output = strings.Repeat("x", 512)
	events := make([]Event, 0)
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "read big"}}, collectEvents(&events)); err != nil {
		t.Fatal(err)
	}
	toolEvent := findEvent(events, EventToolResult)
	if !toolEvent.Result.Truncated || !strings.Contains(toolEvent.Result.Output, toolBudgetMarker) {
		t.Fatalf("tool event result = %+v", toolEvent.Result)
	}
	if len(client.requests) != 2 || len(client.requests[1].Messages) < 3 {
		t.Fatalf("requests = %#v", client.requests)
	}
	if got := client.requests[1].Messages[len(client.requests[1].Messages)-1].Content; !strings.Contains(got, toolBudgetMarker) {
		t.Fatalf("model tool history missing budget marker: %q", got)
	}
}

func TestLoopRequiresWorkspaceEvidenceUntilGrounded(t *testing.T) {
	client := &scriptedClient{streams: []scriptedStreamSpec{
		{events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: model.ToolCall{ID: "ground", Name: "read_file", Arguments: json.RawMessage(`{"path":"README.md"}`)}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishToolCalls},
		}},
		{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "grounded"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithGroundingEvidence(tool.EvidenceWorkspace))
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect repo"}}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(client.requests))
	}
	if client.requests[0].Options.ToolChoice != sdk.ToolChoiceRequired {
		t.Fatalf("first tool choice = %q, want required", client.requests[0].Options.ToolChoice)
	}
	if client.requests[1].Options.ToolChoice != sdk.ToolChoiceAuto {
		t.Fatalf("second tool choice = %q, want auto", client.requests[1].Options.ToolChoice)
	}
}

func TestLoopExplicitReasoningRejectsKnownUnsupportedLevel(t *testing.T) {
	client := &scriptedClient{
		profile: modelprofile.Resolved{ModelID: "limited", Reasoning: modelprofile.Reasoning{Support: modelprofile.SupportYes, Levels: []sdk.ReasoningEffort{sdk.ReasoningLow, sdk.ReasoningMedium}}},
		streams: []scriptedStreamSpec{{events: []sdk.Event{{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}}},
	}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithExplicitReasoningEffort(sdk.ReasoningHigh))
	_, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil)
	if !errors.Is(err, ErrUnsupportedModelCapability) {
		t.Fatalf("Run() error = %v, want unsupported model capability", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("provider requests = %d, want 0", len(client.requests))
	}
}

func TestLoopExplicitReasoningPassesThroughUnknownProfile(t *testing.T) {
	client := &scriptedClient{
		profile: modelprofile.Resolved{ModelID: "future", Reasoning: modelprofile.Reasoning{Support: modelprofile.SupportUnknown}},
		streams: []scriptedStreamSpec{{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "done"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}}},
	}
	loop, _ := newTestLoop(t, client, permission.ActionAllow, WithExplicitReasoningEffort(sdk.ReasoningXHigh))
	if _, err := loop.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: "inspect"}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := client.requests[0].Options.ReasoningEffort; got != sdk.ReasoningXHigh {
		t.Fatalf("ReasoningEffort = %q, want xhigh", got)
	}
}
