package model

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type slowModeBlockingModel struct {
	mu      sync.Mutex
	active  int
	maxSeen int
	started chan *slowModeBlockingStream
}

func (*slowModeBlockingModel) Provider() string { return DefaultOpenCodeName }
func (*slowModeBlockingModel) ModelID() string  { return "test-free" }
func (*slowModeBlockingModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}
func (m *slowModeBlockingModel) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	stream := &slowModeBlockingStream{owner: m, done: make(chan struct{})}
	m.mu.Lock()
	m.active++
	if m.active > m.maxSeen {
		m.maxSeen = m.active
	}
	m.mu.Unlock()
	m.started <- stream
	return stream, nil
}

type slowModeBlockingStream struct {
	owner *slowModeBlockingModel
	done  chan struct{}
	once  sync.Once
}

func (s *slowModeBlockingStream) Next(ctx context.Context) (sdk.Event, error) {
	select {
	case <-s.done:
		return sdk.Event{Kind: sdk.EventFinish, FinishReason: sdk.FinishStop}, nil
	case <-ctx.Done():
		return sdk.Event{}, ctx.Err()
	}
}
func (s *slowModeBlockingStream) Close() error {
	s.once.Do(func() {
		s.owner.mu.Lock()
		s.owner.active--
		s.owner.mu.Unlock()
		close(s.done)
	})
	return nil
}

func testSlowModePolicy() runtimepolicy.OpenCodeFreeSlowModePolicy {
	return runtimepolicy.OpenCodeFreeSlowModePolicy{
		InitialInterval: time.Millisecond,
		MinInterval:     time.Millisecond,
		MaxInterval:     50 * time.Millisecond,
		QueueCapacity:   1,
		MinConcurrency:  1,
		MaxConcurrency:  2,
		HealthySamples:  100,
		PromoteQueue:    1,
		RecoveryPercent: 95,
		BackoffPercent:  200,
	}
}

func TestOpenCodeFreeSlowModeStartsAtOneConcurrentGeneration(t *testing.T) {
	base := &slowModeBlockingModel{started: make(chan *slowModeBlockingStream, 2)}
	policy := testSlowModePolicy()
	policy.QueueCapacity = 2
	controller := newOpenCodeFreeSlowController(policy)
	model := &openCodeFreeSlowModel{base: base, controller: controller}
	streams := make(chan sdk.Stream, 2)

	for range 2 {
		go func() {
			stream, err := model.Stream(context.Background(), sdk.Request{})
			if err != nil {
				t.Errorf("Stream: %v", err)
				return
			}
			streams <- stream
		}()
	}

	first := <-streams
	select {
	case <-streams:
		t.Fatal("second generation started before first released its slot")
	case <-time.After(15 * time.Millisecond):
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second := <-streams
	_ = second.Close()

	base.mu.Lock()
	maxSeen := base.maxSeen
	base.mu.Unlock()
	if maxSeen != 1 {
		t.Fatalf("max concurrent generations = %d, want 1", maxSeen)
	}
}

func TestOpenCodeFreeSlowModeBoundsWaitingQueue(t *testing.T) {
	base := &slowModeBlockingModel{started: make(chan *slowModeBlockingStream, 2)}
	controller := newOpenCodeFreeSlowController(testSlowModePolicy())
	model := &openCodeFreeSlowModel{base: base, controller: controller}
	first, err := model.Stream(context.Background(), sdk.Request{})
	if err != nil {
		t.Fatal(err)
	}

	secondDone := make(chan error, 1)
	go func() {
		stream, streamErr := model.Stream(context.Background(), sdk.Request{})
		if streamErr == nil {
			_ = stream.Close()
		}
		secondDone <- streamErr
	}()
	deadline := time.Now().Add(100 * time.Millisecond)
	for len(controller.admission) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("second request did not enter the bounded queue")
		}
		time.Sleep(time.Millisecond)
	}

	_, err = model.Stream(context.Background(), sdk.Request{})
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "slow_mode_queue_full" || providerErr.Retryable {
		t.Fatalf("queue-full error = %#v", err)
	}
	_ = first.Close()
	if err := <-secondDone; err != nil {
		t.Fatalf("queued request failed: %v", err)
	}
}

type slowModeSequenceModel struct {
	mu     sync.Mutex
	starts []time.Time
	errors []error
}

func (*slowModeSequenceModel) Provider() string { return DefaultOpenCodeName }
func (*slowModeSequenceModel) ModelID() string  { return "test-free" }
func (*slowModeSequenceModel) Capabilities() sdk.ModelCapabilities {
	return sdk.ModelCapabilities{Streaming: true}
}
func (m *slowModeSequenceModel) Stream(context.Context, sdk.Request) (sdk.Stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.starts = append(m.starts, time.Now())
	if len(m.errors) > 0 {
		err := m.errors[0]
		m.errors = m.errors[1:]
		if err != nil {
			return nil, err
		}
	}
	return &emptyRetryTestStream{err: io.EOF}, nil
}

func TestOpenCodeFreeSlowModeBacksOffAfterRateLimit(t *testing.T) {
	policy := testSlowModePolicy()
	policy.InitialInterval = 5 * time.Millisecond
	policy.MinInterval = 5 * time.Millisecond
	policy.MaxInterval = 40 * time.Millisecond
	policy.BackoffPercent = 200
	base := &slowModeSequenceModel{errors: []error{
		sdk.NewProviderError(DefaultOpenCodeName, 429, "rate_limit", "slow down"), nil,
	}}
	model := &openCodeFreeSlowModel{base: base, controller: newOpenCodeFreeSlowController(policy)}

	if _, err := model.Stream(context.Background(), sdk.Request{}); err == nil {
		t.Fatal("first request should be rate limited")
	}
	stream, err := model.Stream(context.Background(), sdk.Request{})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = stream.Next(context.Background())
	_ = stream.Close()

	base.mu.Lock()
	starts := append([]time.Time(nil), base.starts...)
	base.mu.Unlock()
	if len(starts) != 2 {
		t.Fatalf("starts = %d, want 2", len(starts))
	}
	if gap := starts[1].Sub(starts[0]); gap < 9*time.Millisecond {
		t.Fatalf("post-rate-limit gap = %v, want about 10ms or more", gap)
	}
}
