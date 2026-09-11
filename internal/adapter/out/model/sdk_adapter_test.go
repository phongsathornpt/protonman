package model

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestOpenCodeFreeModelFactoryEnablesEmptyStreamRetry(t *testing.T) {
	model := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "nemotron-3.5-lightning-free", WithSessionID("session-1"))
	if _, ok := model.(*emptyStreamRetryModel); !ok {
		t.Fatalf("model type = %T, want *emptyStreamRetryModel", model)
	}
	paid := newSDKOpenAILanguageModel(DefaultOpenCodeName, "https://example.test/v1", "", "paid-model", WithSessionID("session-1"))
	if _, ok := paid.(*emptyStreamRetryModel); ok {
		t.Fatalf("paid model unexpectedly enabled empty stream retry: %T", paid)
	}
}
