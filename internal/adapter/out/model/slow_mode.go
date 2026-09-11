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

type slowModeOutcome uint8

const (
	slowModeSuccess slowModeOutcome = iota
	slowModeFailure
	slowModeCongested
)

type slowModeCompletion struct {
	outcome    slowModeOutcome
	retryAfter time.Duration
}

type slowModeRequest struct {
	ctx   context.Context
	grant chan struct{}
}

type openCodeFreeSlowController struct {
	policy    runtimepolicy.OpenCodeFreeSlowModePolicy
	admission chan struct{}
	requests  chan *slowModeRequest
	done      chan slowModeCompletion
}

var openCodeFreeSlowControllers = struct {
	sync.Mutex
	byRoute map[string]*openCodeFreeSlowController
}{byRoute: make(map[string]*openCodeFreeSlowController)}

func openCodeFreeSlowControllerFor(route string, policy runtimepolicy.OpenCodeFreeSlowModePolicy) *openCodeFreeSlowController {
	openCodeFreeSlowControllers.Lock()
	defer openCodeFreeSlowControllers.Unlock()
	if controller := openCodeFreeSlowControllers.byRoute[route]; controller != nil {
		return controller
	}
	controller := newOpenCodeFreeSlowController(policy)
	openCodeFreeSlowControllers.byRoute[route] = controller
	return controller
}

func newOpenCodeFreeSlowController(policy runtimepolicy.OpenCodeFreeSlowModePolicy) *openCodeFreeSlowController {
	controller := &openCodeFreeSlowController{
		policy:    policy,
		admission: make(chan struct{}, policy.QueueCapacity),
		requests:  make(chan *slowModeRequest),
		done:      make(chan slowModeCompletion, policy.MaxConcurrency+1),
	}
	go controller.run()
	return controller
}

func (c *openCodeFreeSlowController) acquire(ctx context.Context) error {
	select {
	case c.admission <- struct{}{}:
	default:
		return &sdk.ProviderError{
			Provider: DefaultOpenCodeName, Kind: sdk.ErrorOverloaded,
			Code: "slow_mode_queue_full", Message: "free model request queue is full", Retryable: false,
		}
	}

	request := &slowModeRequest{ctx: ctx, grant: make(chan struct{})}
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

func (c *openCodeFreeSlowController) complete(completion slowModeCompletion) {
	c.done <- completion
}

func (c *openCodeFreeSlowController) run() {
	policy := c.policy
	interval := policy.InitialInterval
	limit := policy.MinConcurrency
	healthy := 0
	active := 0
	var pending []*slowModeRequest
	var nextDispatch time.Time

	for {
		pending = pruneSlowModeRequests(pending, c.admission)
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
			case slowModeSuccess:
				healthy++
				if interval > policy.MinInterval {
					interval = scaleSlowModeDuration(interval, policy.RecoveryPercent, policy.MinInterval, policy.MaxInterval)
				}
				if limit < policy.MaxConcurrency && healthy >= policy.HealthySamples && len(pending) >= policy.PromoteQueue {
					limit++
					healthy = 0
				}
			case slowModeCongested:
				healthy = 0
				limit = policy.MinConcurrency
				interval = scaleSlowModeDuration(interval, policy.BackoffPercent, policy.MinInterval, policy.MaxInterval)
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

func pruneSlowModeRequests(pending []*slowModeRequest, admission chan struct{}) []*slowModeRequest {
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

func scaleSlowModeDuration(value time.Duration, percent int, floor, ceiling time.Duration) time.Duration {
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

type openCodeFreeSlowModel struct {
	base       sdk.LanguageModel
	controller *openCodeFreeSlowController
}

func withOpenCodeFreeSlowMode(base sdk.LanguageModel, route string, policy runtimepolicy.OpenCodeFreeSlowModePolicy) sdk.LanguageModel {
	if base == nil {
		return nil
	}
	return &openCodeFreeSlowModel{base: base, controller: openCodeFreeSlowControllerFor(route, policy)}
}

func (m *openCodeFreeSlowModel) Provider() string                    { return m.base.Provider() }
func (m *openCodeFreeSlowModel) ModelID() string                     { return m.base.ModelID() }
func (m *openCodeFreeSlowModel) Capabilities() sdk.ModelCapabilities { return m.base.Capabilities() }
func (m *openCodeFreeSlowModel) ContextWindow() int                  { return sdk.ModelContextWindow(m.base) }
func (m *openCodeFreeSlowModel) TokenLimits() sdk.TokenLimits        { return sdk.ModelTokenLimits(m.base) }

func (m *openCodeFreeSlowModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	if err := m.controller.acquire(ctx); err != nil {
		return nil, err
	}
	stream, err := m.base.Stream(ctx, request)
	if err != nil {
		m.controller.complete(classifySlowModeCompletion(err))
		return nil, err
	}
	if stream == nil {
		m.controller.complete(slowModeCompletion{outcome: slowModeFailure})
		return nil, errors.New("open free model stream: nil stream")
	}
	return &openCodeFreeSlowStream{base: stream, controller: m.controller}, nil
}

type openCodeFreeSlowStream struct {
	base       sdk.Stream
	controller *openCodeFreeSlowController
	once       sync.Once
}

func (s *openCodeFreeSlowStream) Next(ctx context.Context) (sdk.Event, error) {
	event, err := s.base.Next(ctx)
	if err != nil {
		completion := classifySlowModeCompletion(err)
		if errors.Is(err, io.EOF) {
			completion = slowModeCompletion{outcome: slowModeSuccess}
		}
		s.finish(completion)
		return event, err
	}
	if event.Kind == sdk.EventFinish {
		s.finish(slowModeCompletion{outcome: slowModeSuccess})
	}
	return event, nil
}

func (s *openCodeFreeSlowStream) Close() error {
	s.finish(slowModeCompletion{outcome: slowModeFailure})
	return s.base.Close()
}

func (s *openCodeFreeSlowStream) finish(completion slowModeCompletion) {
	s.once.Do(func() { s.controller.complete(completion) })
}

func classifySlowModeCompletion(err error) slowModeCompletion {
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil {
		return slowModeCompletion{outcome: slowModeFailure}
	}
	if providerErr.Kind != sdk.ErrorRateLimit && providerErr.Kind != sdk.ErrorOverloaded {
		return slowModeCompletion{outcome: slowModeFailure}
	}
	completion := slowModeCompletion{outcome: slowModeCongested}
	if providerErr.RateLimit != nil {
		completion.retryAfter = providerErr.RateLimit.RetryAfter
	}
	return completion
}

func openCodeFreeSlowRoute(baseURL, modelID string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(baseURL)), "/") + "|" + strings.ToLower(strings.TrimSpace(modelID))
}
