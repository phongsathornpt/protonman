package model

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type lowConcurrencyOutcome uint8

const (
	lowConcurrencySuccess lowConcurrencyOutcome = iota
	lowConcurrencyFailure
	lowConcurrencyCongested
)

type lowConcurrencyCompletion struct {
	outcome    lowConcurrencyOutcome
	retryAfter time.Duration
}

type lowConcurrencyRequest struct {
	ctx   context.Context
	grant chan struct{}
}

type lowConcurrencyController struct {
	provider  string
	policy    runtimepolicy.LowConcurrencyPolicy
	admission chan struct{}
	requests  chan *lowConcurrencyRequest
	done      chan lowConcurrencyCompletion
}

var lowConcurrencyControllers = struct {
	sync.Mutex
	byRoute map[string]*lowConcurrencyController
}{byRoute: make(map[string]*lowConcurrencyController)}

func lowConcurrencyControllerFor(provider, route string, policy runtimepolicy.LowConcurrencyPolicy) *lowConcurrencyController {
	lowConcurrencyControllers.Lock()
	defer lowConcurrencyControllers.Unlock()
	if controller := lowConcurrencyControllers.byRoute[route]; controller != nil {
		return controller
	}
	controller := newLowConcurrencyController(provider, policy)
	lowConcurrencyControllers.byRoute[route] = controller
	return controller
}

func newLowConcurrencyController(provider string, policy runtimepolicy.LowConcurrencyPolicy) *lowConcurrencyController {
	controller := &lowConcurrencyController{
		provider:  strings.TrimSpace(provider),
		policy:    policy,
		admission: make(chan struct{}, policy.QueueCapacity),
		requests:  make(chan *lowConcurrencyRequest),
		done:      make(chan lowConcurrencyCompletion, policy.MaxConcurrency+1),
	}
	go controller.run()
	return controller
}

func (c *lowConcurrencyController) acquire(ctx context.Context) error {
	select {
	case c.admission <- struct{}{}:
	default:
		return &sdk.ProviderError{
			Provider: c.provider, Kind: sdk.ErrorOverloaded,
			Code: "low_concurrency_queue_full", Message: "low concurrency request queue is full", Retryable: false,
		}
	}

	request := &lowConcurrencyRequest{ctx: ctx, grant: make(chan struct{})}
	select {
	case c.requests <- request:
	case <-ctx.Done():
		<-c.admission
		return ctx.Err()
	}

	select {
	case <-request.grant:
		<-c.admission
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *lowConcurrencyController) complete(completion lowConcurrencyCompletion) {
	c.done <- completion
}

func (c *lowConcurrencyController) run() {
	policy := c.policy
	interval := policy.InitialInterval
	limit := policy.MinConcurrency
	healthy := 0
	active := 0
	var pending []*lowConcurrencyRequest
	var nextDispatch time.Time
	var blockedUntil time.Time

	for {
		pending = pruneLowConcurrencyRequests(pending, c.admission)
		now := time.Now()
		readyAt := nextDispatch
		if blockedUntil.After(readyAt) {
			readyAt = blockedUntil
		}
		if len(pending) > 0 && active < limit && (readyAt.IsZero() || !now.Before(readyAt)) {
			request := pending[0]
			pending = pending[1:]
			select {
			case request.grant <- struct{}{}:
				active++
				nextDispatch = time.Now().Add(interval)
			case <-request.ctx.Done():
				<-c.admission
			}
			continue
		}

		var timer *time.Timer
		var timerC <-chan time.Time
		if len(pending) > 0 && active < limit && !readyAt.IsZero() {
			wait := time.Until(readyAt)
			if wait < 0 {
				wait = 0
			}
			timer = time.NewTimer(wait)
			timerC = timer.C
		}

		select {
		case request := <-c.requests:
			pending = append(pending, request)
		case completion := <-c.done:
			if active > 0 {
				active--
			}
			switch completion.outcome {
			case lowConcurrencySuccess:
				healthy++
				if !blockedUntil.IsZero() && !time.Now().Before(blockedUntil) {
					blockedUntil = time.Time{}
				}
				if interval > policy.MinInterval {
					interval = scaleLowConcurrencyDuration(interval, policy.RecoveryPercent, policy.MinInterval, policy.MaxInterval)
				}
				if limit < policy.MaxConcurrency && healthy >= policy.HealthySamples && len(pending) >= policy.PromoteQueue {
					limit++
					healthy = 0
				}
			case lowConcurrencyCongested:
				healthy = 0
				limit = policy.MinConcurrency
				interval = scaleLowConcurrencyDuration(interval, policy.BackoffPercent, policy.MinInterval, policy.MaxInterval)
				now := time.Now()
				nextDispatch = now.Add(interval)
				if completion.retryAfter > 0 {
					candidate := now.Add(completion.retryAfter)
					if candidate.After(blockedUntil) {
						blockedUntil = candidate
					}
				}
			default:
				healthy = 0
			}
		case <-timerC:
		}
		if timer != nil {
			timer.Stop()
		}
	}
}

func pruneLowConcurrencyRequests(pending []*lowConcurrencyRequest, admission chan struct{}) []*lowConcurrencyRequest {
	kept := pending[:0]
	for _, request := range pending {
		select {
		case <-request.ctx.Done():
			<-admission
		default:
			kept = append(kept, request)
		}
	}
	return kept
}

func scaleLowConcurrencyDuration(value time.Duration, percent int, floor, ceiling time.Duration) time.Duration {
	if percent <= 0 {
		return value
	}
	scaled := time.Duration(int64(value) * int64(percent) / 100)
	if scaled < floor {
		return floor
	}
	if scaled > ceiling {
		return ceiling
	}
	return scaled
}

type lowConcurrencyModel struct {
	base       sdk.LanguageModel
	controller *lowConcurrencyController
}

func withLowConcurrencyMode(base sdk.LanguageModel, provider, route string, policy runtimepolicy.LowConcurrencyPolicy) sdk.LanguageModel {
	if base == nil {
		return nil
	}
	return &lowConcurrencyModel{base: base, controller: lowConcurrencyControllerFor(provider, route, policy)}
}

func (m *lowConcurrencyModel) Provider() string { return m.base.Provider() }
func (m *lowConcurrencyModel) ModelID() string  { return m.base.ModelID() }
func (m *lowConcurrencyModel) Capabilities() sdk.ModelCapabilities {
	return m.base.Capabilities()
}
func (m *lowConcurrencyModel) ContextWindow() int { return sdk.ModelContextWindow(m.base) }
func (m *lowConcurrencyModel) TokenLimits() sdk.TokenLimits {
	return sdk.ModelTokenLimits(m.base)
}

func (m *lowConcurrencyModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	if err := m.controller.acquire(ctx); err != nil {
		return nil, err
	}
	stream, err := m.base.Stream(ctx, request)
	if err != nil {
		m.controller.complete(classifyLowConcurrencyCompletion(err))
		return nil, err
	}
	if stream == nil {
		m.controller.complete(lowConcurrencyCompletion{outcome: lowConcurrencyFailure})
		return nil, errors.New("low concurrency model stream: nil stream")
	}
	return &lowConcurrencyStream{base: stream, controller: m.controller}, nil
}

type lowConcurrencyStream struct {
	base       sdk.Stream
	controller *lowConcurrencyController
	once       sync.Once
}

func (s *lowConcurrencyStream) Next(ctx context.Context) (sdk.Event, error) {
	event, err := s.base.Next(ctx)
	if err != nil {
		completion := classifyLowConcurrencyCompletion(err)
		if errors.Is(err, io.EOF) {
			completion = lowConcurrencyCompletion{outcome: lowConcurrencySuccess}
		}
		s.finish(completion)
		return event, err
	}
	if event.Kind == sdk.EventFinish {
		s.finish(lowConcurrencyCompletion{outcome: lowConcurrencySuccess})
	}
	return event, nil
}

func (s *lowConcurrencyStream) Close() error {
	s.finish(lowConcurrencyCompletion{outcome: lowConcurrencyFailure})
	return s.base.Close()
}

func (s *lowConcurrencyStream) finish(completion lowConcurrencyCompletion) {
	s.once.Do(func() { s.controller.complete(completion) })
}

func classifyLowConcurrencyCompletion(err error) lowConcurrencyCompletion {
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return lowConcurrencyCompletion{outcome: lowConcurrencyFailure}
	}
	if providerErr.Kind != sdk.ErrorRateLimit && providerErr.Kind != sdk.ErrorOverloaded {
		return lowConcurrencyCompletion{outcome: lowConcurrencyFailure}
	}
	completion := lowConcurrencyCompletion{outcome: lowConcurrencyCongested}
	if providerErr.RateLimit != nil {
		completion.retryAfter = providerErr.RateLimit.RetryAfter
	}
	return completion
}

func lowConcurrencyRoute(provider, baseURL, modelID string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "|" + strings.TrimRight(strings.ToLower(strings.TrimSpace(baseURL)), "/") + "|" + strings.ToLower(strings.TrimSpace(modelID))
}
