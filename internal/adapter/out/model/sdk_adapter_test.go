package model

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestOpenCodeSDKAdapterAlwaysSendsClientIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-client"); got != "proton-test" {
			t.Fatalf("x-opencode-client = %q", got)
		}
		if got := r.Header.Get("X-Agent-Type"); got != "protonman" {
			t.Fatalf("X-Agent-Type = %q", got)
		}
		if got := r.Header.Get("X-Agent-Version"); got == "" {
			t.Fatal("X-Agent-Version is empty")
		}
		if got := r.Header.Get("x-opencode-session"); got != "" {
			t.Fatalf("x-opencode-session = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, server.URL, "", "test", WithClientName("proton-test"))
	if model.Provider() != DefaultOpenCodeName {
		t.Fatalf("provider = %q", model.Provider())
	}
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestOpenCodeSDKAdapterSendsSessionIdentityWhenPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-client"); got != "proton" {
			t.Fatalf("x-opencode-client = %q", got)
		}
		if got := r.Header.Get("x-opencode-session"); got != "session-1" {
			t.Fatalf("x-opencode-session = %q", got)
		}
		if got := r.Header.Get("X-Session-Id"); got != "session-1" {
			t.Fatalf("X-Session-Id = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, server.URL, "", "test", WithSessionID("session-1"))
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestAnthropicSDKAdapterSendsSessionIdentityWhenPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Session-Id"); got != "session-anthropic" {
			t.Fatalf("X-Session-Id = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\"}\n\n")
	}))
	defer server.Close()

	model := newSDKAnthropicLanguageModel(server.URL, "", "claude-test", WithSessionID(" session-anthropic "))
	stream, err := model.Stream(context.Background(), sdk.Request{
		Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}},
		Metadata: sdk.RequestMetadata{SessionID: "caller-session"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
}

func TestAgentHeadersIncludeProfile(t *testing.T) {
	cfg := newClientConfig("https://example.com", "", "test")
	WithAgentProfile(" intelligence ")(&cfg)
	headers := agentHeaders(cfg)
	if got := headers.Get("X-Agent-Type"); got != "protonman" {
		t.Fatalf("X-Agent-Type = %q", got)
	}
	if got := headers.Get("X-Agent-Profile"); got != "intelligence" {
		t.Fatalf("X-Agent-Profile = %q", got)
	}
	if got := headers.Get("X-Agent-Version"); got == "" {
		t.Fatal("X-Agent-Version is empty")
	}
}

func TestOpenCodeFreeRetryPreservesSessionIdentity(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if got := r.Header.Get("x-opencode-session"); got != "session-retry" {
			t.Fatalf("attempt %d x-opencode-session = %q", attempts, got)
		}
		if got := r.Header.Get("X-Session-Id"); got != "session-retry" {
			t.Fatalf("attempt %d X-Session-Id = %q", attempts, got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if attempts == 1 {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
			return
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"recovered\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()

	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, server.URL, "", "nemotron-3.5-lightning-free", WithSessionID("session-retry"))
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "recovered" || attempts != 2 {
		t.Fatalf("text=%q attempts=%d, want recovered/2", result.Text, attempts)
	}
}

type emptyRetryTestModel struct {
	streams  []sdk.Stream
	requests int
}

func (*emptyRetryTestModel) Provider() string { return DefaultOpenCodeName }
func (*emptyRetryTestModel) ModelID() string  { return "nemotron-3.5-lightning-free" }
func (*emptyRetryTestModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true}
}
func (m *emptyRetryTestModel) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	m.requests++
	if len(m.streams) == 0 {
		return nil, errors.New("no test stream remains")
	}
	stream := m.streams[0]
	m.streams = m.streams[1:]
	return stream, nil
}

type emptyRetryTestStream struct {
	events []sdk.Event
	err    error
	index  int
}

func (s *emptyRetryTestStream) Next(context.Context) (sdk.Event, error) {
	if s.index < len(s.events) {
		event := s.events[s.index]
		s.index++
		return event, nil
	}
	if s.err != nil {
		return sdk.Event{}, s.err
	}
	return sdk.Event{}, io.EOF
}
func (*emptyRetryTestStream) Close() error { return nil }

func TestEmptyStreamRetryRecoversFromEmptyFinish(t *testing.T) {
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "recovered"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	model := withEmptyStreamRetry(base, 2, 0)
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "recovered" || base.requests != 2 {
		t.Fatalf("result=%q requests=%d, want recovered/2", result.Text, base.requests)
	}
}

func TestEmptyStreamRetryRecoversFromIncompleteStreamBeforeOutput(t *testing.T) {
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{err: sdk.ErrIncompleteStream},
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "ok"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil || result.Text != "ok" || base.requests != 2 {
		t.Fatalf("result=%q requests=%d err=%v", result.Text, base.requests, err)
	}
}

func TestStreamRetryRecoversFromRetryableProviderStreamError(t *testing.T) {
	providerErr := sdk.NewProviderError(DefaultOpenCodeName, 0, "ProviderResponseStreamError", "upstream stream failed")
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{err: providerErr},
		&emptyRetryTestStream{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "recovered"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	stream, err := withStreamRetryPolicy(base, 2, 0, 0, 0, 0).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil || result.Text != "recovered" || base.requests != 2 {
		t.Fatalf("result=%q requests=%d err=%v, want recovered/2", result.Text, base.requests, err)
	}
}

func TestStreamRetryDoesNotReplayRetryableProviderErrorAfterVisibleText(t *testing.T) {
	providerErr := sdk.NewProviderError(DefaultOpenCodeName, 0, "ProviderResponseStreamError", "upstream stream failed")
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "partial"}}, err: providerErr},
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "duplicate"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	stream, err := withStreamRetryPolicy(base, 2, 0, 0, 0, 0).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sdk.CollectStep(context.Background(), stream)
	if !errors.Is(err, providerErr) || base.requests != 1 {
		t.Fatalf("err=%v requests=%d, want original provider error/1", err, base.requests)
	}
}

func TestStreamRetryUsesSDKRetryAfterLimit(t *testing.T) {
	providerErr := sdk.NewProviderError(DefaultOpenCodeName, http.StatusTooManyRequests, "rate_limit_error", "slow down")
	providerErr.RateLimit = &sdk.RateLimitInfo{RetryAfter: 10 * time.Second}
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{err: providerErr},
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "should not replay"}}},
	}}
	policy := streamRetryPolicy(0, 0)
	policy.MaxRetryAfter = 5 * time.Second
	stream, err := withStreamRetryPolicyConfig(base, 2, policy, 0, 0, 0).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	_, err = stream.Next(context.Background())
	if !errors.Is(err, providerErr) || base.requests != 1 {
		t.Fatalf("err=%v requests=%d, want original provider error/1", err, base.requests)
	}
}

func TestEmptyStreamRetryReplaysToolOnlyAttemptWithoutDuplicatingCall(t *testing.T) {
	toolCall := sdk.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventToolCall, ToolCall: toolCall}}, err: sdk.ErrIncompleteStream},
		&emptyRetryTestStream{events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: toolCall},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishToolCalls},
		}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "inspect"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if base.requests != 2 || len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != toolCall.ID {
		t.Fatalf("result=%+v requests=%d, want one replayed tool call across two attempts", result, base.requests)
	}
}

func TestEmptyStreamRetryFlushesBufferedToolCallOnTerminalFinish(t *testing.T) {
	toolCall := sdk.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{events: []sdk.Event{
			{Kind: sdk.EventToolCall, ToolCall: toolCall},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishToolCalls},
		}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "inspect"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if base.requests != 1 || len(result.ToolCalls) != 1 || result.FinishReason != sdk.FinishToolCalls {
		t.Fatalf("result=%+v requests=%d, want one buffered tool call and terminal finish", result, base.requests)
	}
}

func TestEmptyStreamRetryDoesNotReplayAfterVisibleOutput(t *testing.T) {
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "partial"}}, err: sdk.ErrIncompleteStream},
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "duplicate"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sdk.CollectStep(context.Background(), stream)
	if !errors.Is(err, sdk.ErrIncompleteStream) || base.requests != 1 {
		t.Fatalf("err=%v requests=%d, want incomplete/1", err, base.requests)
	}
}

func TestEmptyStreamRetryIsBounded(t *testing.T) {
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "" || result.FinishReason != sdk.FinishStop || base.requests != 3 {
		t.Fatalf("result=%+v requests=%d, want empty stop/3", result, base.requests)
	}
}

func TestEmptyStreamRetryDiscardsUsageFromRetriedAttempt(t *testing.T) {
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{events: []sdk.Event{
			{Kind: sdk.EventUsage, Usage: sdk.Usage{InputTokens: 99, OutputTokens: 0, TotalTokens: 99}},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
		&emptyRetryTestStream{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "ok"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "ok" || result.Usage.TotalTokens != 0 {
		t.Fatalf("result=%+v, want successful attempt without stale usage", result)
	}
}

func TestEmptyStreamRetryHonorsCancellationDuringBackoff(t *testing.T) {
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	stream, err := withEmptyStreamRetry(base, 2, time.Second).Stream(ctx, sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sdk.CollectStep(ctx, stream)
	if !errors.Is(err, context.DeadlineExceeded) || base.requests != 1 {
		t.Fatalf("err=%v requests=%d, want deadline/1", err, base.requests)
	}
}

type noOutputTimeoutTestModel struct {
	attempts  int
	recoverOn int
}

func (*noOutputTimeoutTestModel) Provider() string { return DefaultOpenCodeName }
func (*noOutputTimeoutTestModel) ModelID() string  { return "nemotron-3.5-lightning-free" }
func (*noOutputTimeoutTestModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true}
}
func (m *noOutputTimeoutTestModel) Stream(ctx context.Context, _ sdk.Request) (sdk.Stream, error) {
	m.attempts++
	if m.recoverOn > 0 && m.attempts >= m.recoverOn {
		return &emptyRetryTestStream{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "recovered"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}}, nil
	}
	return &contextWaitTestStream{ctx: ctx}, nil
}

type openTimeoutRetryTestModel struct {
	attempts  int
	recoverOn int
}

func (*openTimeoutRetryTestModel) Provider() string { return DefaultOpenCodeName }
func (*openTimeoutRetryTestModel) ModelID() string  { return "nemotron-3.5-lightning-free" }
func (*openTimeoutRetryTestModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}
func (m *openTimeoutRetryTestModel) Stream(ctx context.Context, _ sdk.Request) (sdk.Stream, error) {
	m.attempts++
	if m.attempts >= m.recoverOn {
		return &emptyRetryTestStream{events: []sdk.Event{
			{Kind: sdk.EventTextDelta, Text: "recovered"},
			{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop},
		}}, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestStreamRetryRecoversWhenOpeningStreamTimesOut(t *testing.T) {
	base := &openTimeoutRetryTestModel{recoverOn: 3}
	model := withStreamRetryPolicy(base, 2, 0, 5*time.Millisecond, 0, 0)
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil || result.Text != "recovered" || base.attempts != 3 {
		t.Fatalf("result=%q attempts=%d err=%v, want recovered/3", result.Text, base.attempts, err)
	}
}

type contextWaitTestStream struct {
	ctx context.Context
}

func (s *contextWaitTestStream) Next(context.Context) (sdk.Event, error) {
	<-s.ctx.Done()
	return sdk.Event{}, s.ctx.Err()
}
func (*contextWaitTestStream) Close() error { return nil }

func TestEmptyStreamRetryRecoversFromNoOutputTimeout(t *testing.T) {
	base := &noOutputTimeoutTestModel{recoverOn: 2}
	model := withEmptyStreamRetryPolicy(base, 2, 0, 5*time.Millisecond)
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "recovered" || base.attempts != 2 {
		t.Fatalf("text=%q attempts=%d, want recovered/2", result.Text, base.attempts)
	}
}

type toolThenWaitStream struct {
	ctx      context.Context
	toolCall sdk.ToolCall
	step     int
}

func (s *toolThenWaitStream) Next(context.Context) (sdk.Event, error) {
	s.step++
	if s.step == 1 {
		return sdk.Event{Kind: sdk.EventToolCall, ToolCall: s.toolCall}, nil
	}
	<-s.ctx.Done()
	return sdk.Event{}, s.ctx.Err()
}
func (*toolThenWaitStream) Close() error { return nil }

type toolIdleRetryModel struct {
	attempts int
	toolCall sdk.ToolCall
}

func (*toolIdleRetryModel) Provider() string { return DefaultOpenCodeName }
func (*toolIdleRetryModel) ModelID() string  { return "nemotron-3.5-lightning-free" }
func (*toolIdleRetryModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true, Tools: true}
}
func (m *toolIdleRetryModel) Stream(ctx context.Context, _ sdk.Request) (sdk.Stream, error) {
	m.attempts++
	if m.attempts == 1 {
		return &toolThenWaitStream{ctx: ctx, toolCall: m.toolCall}, nil
	}
	return &emptyRetryTestStream{events: []sdk.Event{
		{Kind: sdk.EventToolCall, ToolCall: m.toolCall},
		{Kind: sdk.EventFinish, FinishReason: sdk.FinishToolCalls},
	}}, nil
}

func TestStreamRetryRecoversFromIdleBufferedToolAttempt(t *testing.T) {
	call := sdk.ToolCall{ID: "call-idle", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}
	base := &toolIdleRetryModel{toolCall: call}
	model := withStreamRetryPolicy(base, 2, 0, 0, 5*time.Millisecond, 0)
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "inspect"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if base.attempts != 2 || len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != call.ID {
		t.Fatalf("result=%+v attempts=%d, want one tool call after idle retry", result, base.attempts)
	}
}

func TestStreamRetryMaxDurationRecoversReplaySafeAttempt(t *testing.T) {
	base := &noOutputTimeoutTestModel{recoverOn: 2}
	model := withStreamRetryPolicy(base, 2, 0, 0, 0, 5*time.Millisecond)
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "recovered" || base.attempts != 2 {
		t.Fatalf("result=%+v attempts=%d, want max-duration recovery on second attempt", result, base.attempts)
	}
}

func TestEmptyStreamRetryNoOutputTimeoutIsBounded(t *testing.T) {
	base := &noOutputTimeoutTestModel{}
	model := withEmptyStreamRetryPolicy(base, 2, 0, 5*time.Millisecond)
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sdk.CollectStep(context.Background(), stream)
	if !errors.Is(err, sdk.ErrIncompleteStream) || base.attempts != 3 {
		t.Fatalf("err=%v attempts=%d, want incomplete/3", err, base.attempts)
	}
}

type textThenDelayedFinishModel struct{}

func (*textThenDelayedFinishModel) Provider() string { return DefaultOpenCodeName }
func (*textThenDelayedFinishModel) ModelID() string  { return "nemotron-3.5-lightning-free" }
func (*textThenDelayedFinishModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}
func (*textThenDelayedFinishModel) Stream(ctx context.Context, _ sdk.Request) (sdk.Stream, error) {
	return &textThenDelayedFinishStream{ctx: ctx}, nil
}

type textThenDelayedFinishStream struct {
	ctx  context.Context
	step int
}

func (s *textThenDelayedFinishStream) Next(context.Context) (sdk.Event, error) {
	s.step++
	if s.step == 1 {
		return sdk.Event{Kind: sdk.EventTextDelta, Text: "visible"}, nil
	}
	select {
	case <-s.ctx.Done():
		return sdk.Event{}, s.ctx.Err()
	case <-time.After(15 * time.Millisecond):
		return sdk.Event{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}, nil
	}
}
func (*textThenDelayedFinishStream) Close() error { return nil }

func TestEmptyStreamRetryStopsNoOutputWatchdogAfterVisibleOutput(t *testing.T) {
	base := &textThenDelayedFinishModel{}
	model := withEmptyStreamRetryPolicy(base, 2, 0, 5*time.Millisecond)
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(context.Background(), stream)
	if err != nil || result.Text != "visible" || result.FinishReason != sdk.FinishStop {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestStreamIdleTimeoutDoesNotBlindReplayCommittedText(t *testing.T) {
	base := &textThenDelayedFinishModel{}
	model := withStreamRetryPolicy(base, 2, 0, 0, 5*time.Millisecond, 0)
	stream, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = sdk.CollectStep(context.Background(), stream)
	if !errors.Is(err, sdk.ErrIncompleteStream) || !strings.Contains(err.Error(), "idle") {
		t.Fatalf("err=%v, want committed-text idle timeout without replay", err)
	}
}

func TestOpenCodeFreeModelFactoryEnablesEmptyStreamRetry(t *testing.T) {
	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "nemotron-3.5-lightning-free", WithSessionID("session-1"))
	retryModel, ok := model.(*emptyStreamRetryModel)
	if !ok {
		t.Fatalf("model type = %T, want *emptyStreamRetryModel", model)
	}
	if retryModel.maxRetries != runtimepolicy.ModelRetryMaxRetries {
		t.Fatalf("free model max retries = %d, want runtime policy %d", retryModel.maxRetries, runtimepolicy.ModelRetryMaxRetries)
	}
	wantSchedule := runtimepolicy.ModelRetrySchedule()
	if len(retryModel.retryPolicy.RetryDelays) != len(wantSchedule) {
		t.Fatalf("free model retry schedule = %v, want %v", retryModel.retryPolicy.RetryDelays, wantSchedule)
	}
	for index, want := range wantSchedule {
		if got := retryModel.retryPolicy.RetryDelays[index]; got != want {
			t.Fatalf("free model retry delay %d = %v, want %v", index+1, got, want)
		}
	}
	paid := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "paid-model", WithSessionID("session-1"))
	if _, ok := paid.(*emptyStreamRetryModel); ok {
		t.Fatalf("paid model unexpectedly enabled empty stream retry: %T", paid)
	}
}

func TestLowConcurrencySettingControlsOpenCodeWrapper(t *testing.T) {
	freeAuto := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "nemotron-3.5-lightning-free")
	retryAuto, ok := freeAuto.(*emptyStreamRetryModel)
	if !ok {
		t.Fatalf("free auto type = %T, want retry wrapper", freeAuto)
	}
	if _, ok := retryAuto.base.(*openCodeFreeLowConcurrencyModel); !ok {
		t.Fatalf("free auto inner type = %T, want low concurrency wrapper", retryAuto.base)
	}

	freeOff := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "nemotron-3.5-lightning-free", WithLowConcurrencyMode(LowConcurrencyOff))
	retryOff, ok := freeOff.(*emptyStreamRetryModel)
	if !ok {
		t.Fatalf("free off type = %T, want retry wrapper", freeOff)
	}
	if _, ok := retryOff.base.(*openCodeFreeLowConcurrencyModel); ok {
		t.Fatalf("free off unexpectedly retained low concurrency wrapper: %T", retryOff.base)
	}

	paidAuto := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "paid-model")
	if _, ok := paidAuto.(*openCodeFreeLowConcurrencyModel); ok {
		t.Fatalf("paid auto unexpectedly enabled low concurrency: %T", paidAuto)
	}

	paidOn := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "paid-model", WithLowConcurrencyMode(LowConcurrencyOn))
	if _, ok := paidOn.(*openCodeFreeLowConcurrencyModel); !ok {
		t.Fatalf("paid on type = %T, want low concurrency wrapper", paidOn)
	}

	nonOpenCode := newSDKOpenAILanguageModel(DefaultOpenAIName, "https://api.openai.com/v1", "key", "gpt-test", WithLowConcurrencyMode(LowConcurrencyOn))
	if _, ok := nonOpenCode.(*openCodeFreeLowConcurrencyModel); ok {
		t.Fatalf("non-OpenCode model unexpectedly enabled low concurrency: %T", nonOpenCode)
	}
}

func TestOpenCodeFreeFactoryUsesOneRetryBudget(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"type":"rate_limit_error","message":"slow down"}}`)
	}))
	defer server.Close()

	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, server.URL, "", "nemotron-3.5-lightning-free")
	_, err := model.Stream(context.Background(), sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err == nil || attempts != runtimepolicy.ModelRetryMaxRetries+1 {
		t.Fatalf("err=%v attempts=%d, want error after %d total attempts", err, attempts, runtimepolicy.ModelRetryMaxRetries+1)
	}
}

func TestEmptyStreamRetryPublishesCountdownMetadata(t *testing.T) {
	base := &emptyRetryTestModel{streams: []sdk.Stream{
		&emptyRetryTestStream{err: sdk.ErrIncompleteStream},
		&emptyRetryTestStream{err: sdk.ErrIncompleteStream},
		&emptyRetryTestStream{events: []sdk.Event{{Kind: sdk.EventTextDelta, Text: "ok"}, {Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}}},
	}}
	var retries []sdk.RetryEvent
	ctx := sdk.WithRetryObserver(context.Background(), func(_ context.Context, event sdk.RetryEvent) {
		retries = append(retries, event)
	})
	stream, err := withStreamRetryPolicyAndGap(base, 2, 5*time.Millisecond, 10*time.Millisecond, 0, 0, 0).Stream(ctx, sdk.Request{Messages: []sdk.Message{{Role: sdk.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := sdk.CollectStep(ctx, stream)
	if err != nil || result.Text != "ok" {
		t.Fatalf("result=%q err=%v", result.Text, err)
	}
	if len(retries) != 2 {
		t.Fatalf("retry events = %+v, want two", retries)
	}
	if got := retries[0]; got.Provider != DefaultOpenCodeName || got.Reason != "incomplete_stream" || got.Attempt != 1 || got.MaxRetries != 2 || got.Delay != 5*time.Millisecond || got.RetryAt.IsZero() {
		t.Fatalf("retry 1 event = %+v", got)
	}
	if got := retries[1]; got.Attempt != 2 || got.Delay != 20*time.Millisecond || got.RetryAt.IsZero() {
		t.Fatalf("retry 2 event = %+v, want 20ms cooldown", got)
	}
}
