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

type openCodeFreeLowConcurrencyController struct {
	policy    runtimepolicy.OpenCodeFreeLowConcurrencyPolicy
	admission chan struct{}
	requests  chan *lowConcurrencyRequest
	done      chan lowConcurrencyCompletion
}

var openCodeFreeLowConcurrencyControllers = struct {
	sync.Mutex
	byRoute map[string]*openCodeFreeLowConcurrencyController
}{byRoute: make(map[string]*openCodeFreeLowConcurrencyController)}

func openCodeFreeLowConcurrencyControllerFor(route string, policy runtimepolicy.OpenCodeFreeLowConcurrencyPolicy) *openCodeFreeLowConcurrencyController {
	openCodeFreeLowConcurrencyControllers.Lock()
	defer openCodeFreeLowConcurrencyControllers.Unlock()
	if controller := openCodeFreeLowConcurrencyControllers.byRoute[route]; controller != nil {
		return controller
	}
	controller := newOpenCodeFreeLowConcurrencyController(policy)
	openCodeFreeLowConcurrencyControllers.byRoute[route] = controller
	return controller
}

func newOpenCodeFreeLowConcurrencyController(policy runtimepolicy.OpenCodeFreeLowConcurrencyPolicy) *openCodeFreeLowConcurrencyController {
	controller := &openCodeFreeLowConcurrencyController{
		policy:    policy,
		admission: make(chan struct{}, policy.QueueCapacity),
		requests:  make(chan *lowConcurrencyRequest),
		done:      make(chan lowConcurrencyCompletion, policy.MaxConcurrency+1),
	}
	go controller.run()
	return controller
}

func (c *openCodeFreeLowConcurrencyController) acquire(ctx context.Context) error {
	select {
	case c.admission <- struct{}{}:
	default:
		return &sdk.ProviderError{
			Provider: DefaultOpenCodeName, Kind: sdk.ErrorOverloaded,
			Code: "low_concurrency_queue_full", Message: "free model request queue is full", Retryable: false,
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

func (c *openCodeFreeLowConcurrencyController) complete(completion lowConcurrencyCompletion) {
	c.done <- completion
}

func (c *openCodeFreeLowConcurrencyController) run() {
	policy := c.policy
	interval := policy.InitialInterval
	limit := policy.MinConcurrency
	healthy := 0
	active := 0
	var pending []*lowConcurrencyRequest
	var nextDispatch time.Time

	for {
		pending = pruneLowConcurrencyRequests(pending, c.admission)
		if len(pending) > 0 && active < limit && (nextDispatch.IsZero() || !time.Now().Before(nextDispatch)) {
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
		if len(pending) > 0 && active < limit && !nextDispatch.IsZero() {
			wait := time.Until(nextDispatch)
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
				if completion.retryAfter > interval {
					interval = min(completion.retryAfter, policy.MaxInterval)
				}
				nextDispatch = time.Now().Add(interval)
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
	for len(pending) > 0 {
		select {
		case <-pending[0].ctx.Done():
			<-admission
			pending = pending[1:]
		default:
			return pending
		}
	}
	return pending
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

type openCodeFreeLowConcurrencyModel struct {
	base       sdk.LanguageModel
	controller *openCodeFreeLowConcurrencyController
}

func withOpenCodeFreeLowConcurrencyMode(base sdk.LanguageModel, route string, policy runtimepolicy.OpenCodeFreeLowConcurrencyPolicy) sdk.LanguageModel {
	if base == nil {
		return nil
	}
	return &openCodeFreeLowConcurrencyModel{base: base, controller: openCodeFreeLowConcurrencyControllerFor(route, policy)}
}

func (m *openCodeFreeLowConcurrencyModel) Provider() string { return m.base.Provider() }
func (m *openCodeFreeLowConcurrencyModel) ModelID() string  { return m.base.ModelID() }
func (m *openCodeFreeLowConcurrencyModel) Capabilities() sdk.ModelCapabilities {
	return m.base.Capabilities()
}
func (m *openCodeFreeLowConcurrencyModel) ContextWindow() int { return sdk.ModelContextWindow(m.base) }
func (m *openCodeFreeLowConcurrencyModel) TokenLimits() sdk.TokenLimits {
	return sdk.ModelTokenLimits(m.base)
}

func (m *openCodeFreeLowConcurrencyModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
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
		return nil, errors.New("open free model stream: nil stream")
	}
	return &openCodeFreeLowConcurrencyStream{base: stream, controller: m.controller}, nil
}

type openCodeFreeLowConcurrencyStream struct {
	base       sdk.Stream
	controller *openCodeFreeLowConcurrencyController
	once       sync.Once
}

func (s *openCodeFreeLowConcurrencyStream) Next(ctx context.Context) (sdk.Event, error) {
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

func (s *openCodeFreeLowConcurrencyStream) Close() error {
	s.finish(lowConcurrencyCompletion{outcome: lowConcurrencyFailure})
	return s.base.Close()
}

func (s *openCodeFreeLowConcurrencyStream) finish(completion lowConcurrencyCompletion) {
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

func openCodeFreeLowConcurrencyRoute(baseURL, modelID string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(baseURL)), "/") + "|" + strings.ToLower(strings.TrimSpace(modelID))
}
