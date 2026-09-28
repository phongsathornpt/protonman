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
	domain "github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	port "github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
	usecase "github.com/phongsathornpt/protonman/pkg/proton-sdk/usecase"
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
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
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
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
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

	model := newSDKAnthropicLanguageModel("anthropic", server.URL, "", "claude-test", WithSessionID(" session-anthropic "))
	stream, err := model.Stream(context.Background(), domain.Request{
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}},
		Metadata: domain.RequestMetadata{SessionID: "caller-session"},
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
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "recovered" || attempts != 2 {
		t.Fatalf("text=%q attempts=%d, want recovered/2", result.Text, attempts)
	}
}

type emptyRetryTestModel struct {
	streams  []port.Stream
	requests int
}

func (*emptyRetryTestModel) Provider() string { return DefaultOpenCodeName }
func (*emptyRetryTestModel) ModelID() string  { return "nemotron-3.5-lightning-free" }
func (*emptyRetryTestModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true, Tools: true}
}
func (m *emptyRetryTestModel) Stream(context.Context, domain.Request) (port.Stream, error) {
	m.requests++
	if len(m.streams) == 0 {
		return nil, errors.New("no test stream remains")
	}
	stream := m.streams[0]
	m.streams = m.streams[1:]
	return stream, nil
}

type emptyRetryTestStream struct {
	events []domain.Event
	err    error
	index  int
}

func (s *emptyRetryTestStream) Next(context.Context) (domain.Event, error) {
	if s.index < len(s.events) {
		event := s.events[s.index]
		s.index++
		return event, nil
	}
	if s.err != nil {
		return domain.Event{}, s.err
	}
	return domain.Event{}, io.EOF
}
func (*emptyRetryTestStream) Close() error { return nil }

func TestEmptyStreamRetryRecoversFromEmptyFinish(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "recovered"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	model := withEmptyStreamRetry(base, 2, 0)
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "recovered" || base.requests != 2 {
		t.Fatalf("result=%q requests=%d, want recovered/2", result.Text, base.requests)
	}
}

func TestEmptyStreamRetryRecoversFromIncompleteStreamBeforeOutput(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{err: domain.ErrIncompleteStream},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "ok"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil || result.Text != "ok" || base.requests != 2 {
		t.Fatalf("result=%q requests=%d err=%v", result.Text, base.requests, err)
	}
}

func TestStreamRetryRecoversFromRetryableProviderStreamError(t *testing.T) {
	providerErr := domain.NewProviderError(DefaultOpenCodeName, 0, "ProviderResponseStreamError", "upstream stream failed")
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{err: providerErr},
		&emptyRetryTestStream{events: []domain.Event{
			{Kind: domain.EventTextDelta, Text: "recovered"},
			{Kind: domain.EventFinish, FinishReason: domain.FinishStop},
		}},
	}}
	stream, err := withStreamRetryPolicy(base, 2, 0, 0, 0, 0).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil || result.Text != "recovered" || base.requests != 2 {
		t.Fatalf("result=%q requests=%d err=%v, want recovered/2", result.Text, base.requests, err)
	}
}

func TestStreamRetryDoesNotReplayRetryableProviderErrorAfterVisibleText(t *testing.T) {
	providerErr := domain.NewProviderError(DefaultOpenCodeName, 0, "ProviderResponseStreamError", "upstream stream failed")
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "partial"}}, err: providerErr},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "duplicate"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	stream, err := withStreamRetryPolicy(base, 2, 0, 0, 0, 0).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.CollectStep(context.Background(), stream)
	if !errors.Is(err, providerErr) || base.requests != 1 {
		t.Fatalf("err=%v requests=%d, want original provider error/1", err, base.requests)
	}
}

func TestStreamRetryUsesSDKRetryAfterLimit(t *testing.T) {
	providerErr := domain.NewProviderError(DefaultOpenCodeName, http.StatusTooManyRequests, "rate_limit_error", "slow down")
	providerErr.RateLimit = &domain.RateLimitInfo{RetryAfter: 10 * time.Second}
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{err: providerErr},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "should not replay"}}},
	}}
	policy := streamRetryPolicy(0, 0)
	policy.MaxRetryAfter = 5 * time.Second
	stream, err := withStreamRetryPolicyConfig(base, 2, policy, 0, 0, 0).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
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
	toolCall := domain.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventToolCall, ToolCall: toolCall}}, err: domain.ErrIncompleteStream},
		&emptyRetryTestStream{events: []domain.Event{
			{Kind: domain.EventToolCall, ToolCall: toolCall},
			{Kind: domain.EventFinish, FinishReason: domain.FinishToolCalls},
		}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "inspect"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if base.requests != 2 || len(result.ToolCalls) != 1 || result.ToolCalls[0].ID != toolCall.ID {
		t.Fatalf("result=%+v requests=%d, want one replayed tool call across two attempts", result, base.requests)
	}
}

func TestEmptyStreamRetryFlushesBufferedToolCallOnTerminalFinish(t *testing.T) {
	toolCall := domain.ToolCall{ID: "call-1", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{
			{Kind: domain.EventToolCall, ToolCall: toolCall},
			{Kind: domain.EventFinish, FinishReason: domain.FinishToolCalls},
		}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "inspect"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if base.requests != 1 || len(result.ToolCalls) != 1 || result.FinishReason != domain.FinishToolCalls {
		t.Fatalf("result=%+v requests=%d, want one buffered tool call and terminal finish", result, base.requests)
	}
}

func TestEmptyStreamRetryDoesNotReplayAfterVisibleOutput(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "partial"}}, err: domain.ErrIncompleteStream},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "duplicate"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.CollectStep(context.Background(), stream)
	if !errors.Is(err, domain.ErrIncompleteStream) || base.requests != 1 {
		t.Fatalf("err=%v requests=%d, want incomplete/1", err, base.requests)
	}
}

func TestEmptyStreamRetryIsBounded(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "" || result.FinishReason != domain.FinishStop || base.requests != 3 {
		t.Fatalf("result=%+v requests=%d, want empty stop/3", result, base.requests)
	}
}

func TestEmptyStreamRetryDiscardsUsageFromRetriedAttempt(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{
			{Kind: domain.EventUsage, Usage: domain.Usage{InputTokens: 99, OutputTokens: 0, TotalTokens: 99}},
			{Kind: domain.EventFinish, FinishReason: domain.FinishStop},
		}},
		&emptyRetryTestStream{events: []domain.Event{
			{Kind: domain.EventTextDelta, Text: "ok"},
			{Kind: domain.EventFinish, FinishReason: domain.FinishStop},
		}},
	}}
	stream, err := withEmptyStreamRetry(base, 2, 0).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "ok" || result.Usage.TotalTokens != 0 {
		t.Fatalf("result=%+v, want successful attempt without stale usage", result)
	}
}

func TestEmptyStreamRetryHonorsCancellationDuringBackoff(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	stream, err := withEmptyStreamRetry(base, 2, time.Second).Stream(ctx, domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.CollectStep(ctx, stream)
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
func (*noOutputTimeoutTestModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true, Tools: true}
}
func (m *noOutputTimeoutTestModel) Stream(ctx context.Context, _ domain.Request) (port.Stream, error) {
	m.attempts++
	if m.recoverOn > 0 && m.attempts >= m.recoverOn {
		return &emptyRetryTestStream{events: []domain.Event{
			{Kind: domain.EventTextDelta, Text: "recovered"},
			{Kind: domain.EventFinish, FinishReason: domain.FinishStop},
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
func (*openTimeoutRetryTestModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true}
}
func (m *openTimeoutRetryTestModel) Stream(ctx context.Context, _ domain.Request) (port.Stream, error) {
	m.attempts++
	if m.attempts >= m.recoverOn {
		return &emptyRetryTestStream{events: []domain.Event{
			{Kind: domain.EventTextDelta, Text: "recovered"},
			{Kind: domain.EventFinish, FinishReason: domain.FinishStop},
		}}, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestStreamRetryRecoversWhenOpeningStreamTimesOut(t *testing.T) {
	base := &openTimeoutRetryTestModel{recoverOn: 3}
	model := withStreamRetryPolicy(base, 2, 0, 5*time.Millisecond, 0, 0)
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil || result.Text != "recovered" || base.attempts != 3 {
		t.Fatalf("result=%q attempts=%d err=%v, want recovered/3", result.Text, base.attempts, err)
	}
}

type contextWaitTestStream struct {
	ctx context.Context
}

func (s *contextWaitTestStream) Next(context.Context) (domain.Event, error) {
	<-s.ctx.Done()
	return domain.Event{}, s.ctx.Err()
}
func (*contextWaitTestStream) Close() error { return nil }

func TestEmptyStreamRetryRecoversFromNoOutputTimeout(t *testing.T) {
	base := &noOutputTimeoutTestModel{recoverOn: 2}
	model := withEmptyStreamRetryPolicy(base, 2, 0, 5*time.Millisecond)
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "recovered" || base.attempts != 2 {
		t.Fatalf("text=%q attempts=%d, want recovered/2", result.Text, base.attempts)
	}
}

type toolThenWaitStream struct {
	ctx      context.Context
	toolCall domain.ToolCall
	step     int
}

func (s *toolThenWaitStream) Next(context.Context) (domain.Event, error) {
	s.step++
	if s.step == 1 {
		return domain.Event{Kind: domain.EventToolCall, ToolCall: s.toolCall}, nil
	}
	<-s.ctx.Done()
	return domain.Event{}, s.ctx.Err()
}
func (*toolThenWaitStream) Close() error { return nil }

type toolIdleRetryModel struct {
	attempts int
	toolCall domain.ToolCall
}

func (*toolIdleRetryModel) Provider() string { return DefaultOpenCodeName }
func (*toolIdleRetryModel) ModelID() string  { return "nemotron-3.5-lightning-free" }
func (*toolIdleRetryModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true, Tools: true}
}
func (m *toolIdleRetryModel) Stream(ctx context.Context, _ domain.Request) (port.Stream, error) {
	m.attempts++
	if m.attempts == 1 {
		return &toolThenWaitStream{ctx: ctx, toolCall: m.toolCall}, nil
	}
	return &emptyRetryTestStream{events: []domain.Event{
		{Kind: domain.EventToolCall, ToolCall: m.toolCall},
		{Kind: domain.EventFinish, FinishReason: domain.FinishToolCalls},
	}}, nil
}

func TestStreamRetryRecoversFromIdleBufferedToolAttempt(t *testing.T) {
	call := domain.ToolCall{ID: "call-idle", Name: "read", Arguments: json.RawMessage(`{"path":"README.md"}`)}
	base := &toolIdleRetryModel{toolCall: call}
	model := withStreamRetryPolicy(base, 2, 0, 0, 5*time.Millisecond, 0)
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "inspect"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
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
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
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
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.CollectStep(context.Background(), stream)
	if !errors.Is(err, domain.ErrIncompleteStream) || base.attempts != 3 {
		t.Fatalf("err=%v attempts=%d, want incomplete/3", err, base.attempts)
	}
}

type textThenDelayedFinishModel struct{}

func (*textThenDelayedFinishModel) Provider() string { return DefaultOpenCodeName }
func (*textThenDelayedFinishModel) ModelID() string  { return "nemotron-3.5-lightning-free" }
func (*textThenDelayedFinishModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true}
}
func (*textThenDelayedFinishModel) Stream(ctx context.Context, _ domain.Request) (port.Stream, error) {
	return &textThenDelayedFinishStream{ctx: ctx}, nil
}

type textThenDelayedFinishStream struct {
	ctx  context.Context
	step int
}

func (s *textThenDelayedFinishStream) Next(context.Context) (domain.Event, error) {
	s.step++
	if s.step == 1 {
		return domain.Event{Kind: domain.EventTextDelta, Text: "visible"}, nil
	}
	select {
	case <-s.ctx.Done():
		return domain.Event{}, s.ctx.Err()
	case <-time.After(15 * time.Millisecond):
		return domain.Event{Kind: domain.EventFinish, FinishReason: domain.FinishStop}, nil
	}
}
func (*textThenDelayedFinishStream) Close() error { return nil }

func TestEmptyStreamRetryStopsNoOutputWatchdogAfterVisibleOutput(t *testing.T) {
	base := &textThenDelayedFinishModel{}
	model := withEmptyStreamRetryPolicy(base, 2, 0, 5*time.Millisecond)
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil || result.Text != "visible" || result.FinishReason != domain.FinishStop {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestStreamIdleTimeoutDoesNotBlindReplayCommittedText(t *testing.T) {
	base := &textThenDelayedFinishModel{}
	model := withStreamRetryPolicy(base, 2, 0, 0, 5*time.Millisecond, 0)
	stream, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.CollectStep(context.Background(), stream)
	if !errors.Is(err, domain.ErrIncompleteStream) || !strings.Contains(err.Error(), "idle") {
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
	paidRetry, ok := paid.(*emptyStreamRetryModel)
	if !ok {
		t.Fatalf("paid model unexpectedly missing replay-safe retry wrapper: %T", paid)
	}
	if paidRetry.maxRetries != runtimepolicy.ModelStreamReplayMaxRetries {
		t.Fatalf("paid model max retries = %d, want runtime policy %d", paidRetry.maxRetries, runtimepolicy.ModelStreamReplayMaxRetries)
	}
	if paidRetry.retryOpenFailures {
		t.Fatal("paid model wrapper must not retry stream opens")
	}
}

func TestLowConcurrencySettingControlsOpenCodeWrapper(t *testing.T) {
	freeAuto := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "nemotron-3.5-lightning-free")
	retryAuto, ok := freeAuto.(*emptyStreamRetryModel)
	if !ok {
		t.Fatalf("free auto type = %T, want retry wrapper", freeAuto)
	}
	if _, ok := retryAuto.base.(*lowConcurrencyModel); !ok {
		t.Fatalf("free auto inner type = %T, want low concurrency wrapper", retryAuto.base)
	}

	freeOff := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "nemotron-3.5-lightning-free", WithLowConcurrencyMode(LowConcurrencyOff))
	retryOff, ok := freeOff.(*emptyStreamRetryModel)
	if !ok {
		t.Fatalf("free off type = %T, want retry wrapper", freeOff)
	}
	if _, ok := retryOff.base.(*lowConcurrencyModel); ok {
		t.Fatalf("free off unexpectedly retained low concurrency wrapper: %T", retryOff.base)
	}

	paidAuto := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "paid-model")
	if inner := unwrapRetryModel(paidAuto); inner != nil {
		if _, ok := inner.(*lowConcurrencyModel); ok {
			t.Fatalf("paid auto unexpectedly enabled low concurrency: %T", inner)
		}
	} else if _, ok := paidAuto.(*lowConcurrencyModel); ok {
		t.Fatalf("paid auto unexpectedly enabled low concurrency: %T", paidAuto)
	}

	paidOn := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "paid-model", WithLowConcurrencyMode(LowConcurrencyOn))
	if inner := unwrapRetryModel(paidOn); inner == nil {
		t.Fatalf("paid on type = %T, want replay wrapper around low concurrency wrapper", paidOn)
	} else if _, ok := inner.(*lowConcurrencyModel); !ok {
		t.Fatalf("paid on inner type = %T, want low concurrency wrapper", inner)
	}

	nonOpenCode := newSDKOpenAILanguageModel(DefaultOpenAIName, "https://api.openai.com/v1", "key", "gpt-test", WithLowConcurrencyMode(LowConcurrencyOn))
	if inner := unwrapRetryModel(nonOpenCode); inner == nil {
		t.Fatalf("non-OpenCode on type = %T, want replay wrapper around low concurrency wrapper", nonOpenCode)
	} else if _, ok := inner.(*lowConcurrencyModel); !ok {
		t.Fatalf("non-OpenCode on inner type = %T, want low concurrency wrapper", inner)
	}

	anthropicOn := newSDKAnthropicLanguageModel(DefaultAnthropicName, "https://api.anthropic.com", "key", "claude-test", WithLowConcurrencyMode(LowConcurrencyOn))
	if inner := unwrapRetryModel(anthropicOn); inner == nil {
		t.Fatalf("anthropic on type = %T, want replay wrapper around low concurrency wrapper", anthropicOn)
	} else if _, ok := inner.(*lowConcurrencyModel); !ok {
		t.Fatalf("anthropic on inner type = %T, want low concurrency wrapper", inner)
	}
}

func unwrapRetryModel(model port.LanguageModel) port.LanguageModel {
	if retry, ok := model.(*emptyStreamRetryModel); ok && retry != nil {
		return retry.base
	}
	return nil
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
	_, err := model.Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err == nil || attempts != runtimepolicy.ModelRetryMaxRetries+1 {
		t.Fatalf("err=%v attempts=%d, want error after %d total attempts", err, attempts, runtimepolicy.ModelRetryMaxRetries+1)
	}
}

func TestReplaySafeRetryRecoversFromIncompleteBeforeOutput(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{err: domain.ErrIncompleteStream},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "ok"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	policy := streamRetryPolicy(0, 0)
	stream, err := withReplaySafeRetryConfig(base, 2, policy).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil || result.Text != "ok" || base.requests != 2 {
		t.Fatalf("result=%q requests=%d err=%v, want ok/2", result.Text, base.requests, err)
	}
}

func TestReplaySafeRetryDoesNotReplayAfterVisibleText(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "partial"}}, err: domain.ErrIncompleteStream},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "duplicate"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	policy := streamRetryPolicy(0, 0)
	stream, err := withReplaySafeRetryConfig(base, 2, policy).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.CollectStep(context.Background(), stream)
	if !errors.Is(err, domain.ErrIncompleteStream) || base.requests != 1 {
		t.Fatalf("err=%v requests=%d, want incomplete/1", err, base.requests)
	}
}

func TestReplaySafeRetryDoesNotRetryOpenFailure(t *testing.T) {
	providerErr := domain.NewProviderError(DefaultOpenAIName, 0, "ProviderResponseStreamError", "upstream open failed")
	base := &openFailTestModel{err: providerErr}
	policy := streamRetryPolicy(0, 0)
	_, err := withReplaySafeRetryConfig(base, 2, policy).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if !errors.Is(err, providerErr) || base.requests != 1 {
		t.Fatalf("err=%v requests=%d, want original open error/1", err, base.requests)
	}
}

type openFailTestModel struct {
	err      error
	requests int
}

func (*openFailTestModel) Provider() string { return DefaultOpenAIName }
func (*openFailTestModel) ModelID() string  { return "paid-model" }
func (*openFailTestModel) Capabilities() domain.ModelCapabilities {
	return domain.ModelCapabilities{Streaming: true, Tools: true}
}
func (m *openFailTestModel) Stream(context.Context, domain.Request) (port.Stream, error) {
	m.requests++
	return nil, m.err
}

func TestReplaySafeRetryIsBounded(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	policy := streamRetryPolicy(0, 0)
	stream, err := withReplaySafeRetryConfig(base, 2, policy).Stream(context.Background(), domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(context.Background(), stream)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "" || base.requests != 3 {
		t.Fatalf("result=%+v requests=%d, want empty/3", result, base.requests)
	}
}

func TestGenericFactoryEnablesReplaySafeRetry(t *testing.T) {
	paid := newSDKOpenAILanguageModel(DefaultOpenAIName, "https://api.openai.com/v1", "key", "gpt-test")
	retry, ok := paid.(*emptyStreamRetryModel)
	if !ok {
		t.Fatalf("paid OpenAI type = %T, want *emptyStreamRetryModel", paid)
	}
	if retry.maxRetries != runtimepolicy.ModelStreamReplayMaxRetries {
		t.Fatalf("paid OpenAI max retries = %d, want %d", retry.maxRetries, runtimepolicy.ModelStreamReplayMaxRetries)
	}
	if retry.retryOpenFailures {
		t.Fatal("paid OpenAI wrapper must not retry stream opens")
	}
	if retry.firstEventTimeout != 0 || retry.idleEventTimeout != 0 || retry.maxStreamDuration != 0 {
		t.Fatalf("paid OpenAI wrapper must not arm stream timeouts: %+v", retry)
	}

	anthropic := newSDKAnthropicLanguageModel(DefaultAnthropicName, "https://api.anthropic.com", "key", "claude-test")
	anthropicRetry, ok := anthropic.(*emptyStreamRetryModel)
	if !ok {
		t.Fatalf("anthropic type = %T, want *emptyStreamRetryModel", anthropic)
	}
	if anthropicRetry.maxRetries != runtimepolicy.ModelStreamReplayMaxRetries || anthropicRetry.retryOpenFailures {
		t.Fatalf("anthropic wrapper = %+v, want generic replay-safe budget without open retries", anthropicRetry)
	}

	free := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "nemotron-3.5-lightning-free")
	freeRetry, ok := free.(*emptyStreamRetryModel)
	if !ok {
		t.Fatalf("free type = %T, want *emptyStreamRetryModel", free)
	}
	if !freeRetry.retryOpenFailures || freeRetry.maxRetries != runtimepolicy.ModelRetryMaxRetries {
		t.Fatalf("free wrapper = %+v, want open-retry ownership with full budget", freeRetry)
	}
}

func TestEmptyStreamRetryPublishesCountdownMetadata(t *testing.T) {
	base := &emptyRetryTestModel{streams: []port.Stream{
		&emptyRetryTestStream{err: domain.ErrIncompleteStream},
		&emptyRetryTestStream{err: domain.ErrIncompleteStream},
		&emptyRetryTestStream{events: []domain.Event{{Kind: domain.EventTextDelta, Text: "ok"}, {Kind: domain.EventFinish, FinishReason: domain.FinishStop}}},
	}}
	var retries []domain.RetryEvent
	ctx := domain.WithRetryObserver(context.Background(), func(_ context.Context, event domain.RetryEvent) {
		retries = append(retries, event)
	})
	stream, err := withStreamRetryPolicyAndGap(base, 2, 5*time.Millisecond, 10*time.Millisecond, 0, 0, 0).Stream(ctx, domain.Request{Messages: []domain.Message{{Role: domain.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := usecase.CollectStep(ctx, stream)
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
