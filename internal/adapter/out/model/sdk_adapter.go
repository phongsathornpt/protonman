package model

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
	sdkanthropic "github.com/phongsathornpt/protonman/proton-sdk/provider/anthropic"
	sdkopenai "github.com/phongsathornpt/protonman/proton-sdk/provider/openai"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

type sessionBoundModel struct {
	base      port.LanguageModel
	sessionID string
}

func withSessionID(base port.LanguageModel, sessionID string) port.LanguageModel {
	sessionID = strings.TrimSpace(sessionID)
	if base == nil || sessionID == "" {
		return base
	}
	return &sessionBoundModel{base: base, sessionID: sessionID}
}

func (m *sessionBoundModel) Provider() string                       { return m.base.Provider() }
func (m *sessionBoundModel) ModelID() string                        { return m.base.ModelID() }
func (m *sessionBoundModel) Capabilities() domain.ModelCapabilities { return m.base.Capabilities() }
func (m *sessionBoundModel) ContextWindow() int                     { return usecase.ModelContextWindow(m.base) }
func (m *sessionBoundModel) TokenLimits() domain.TokenLimits        { return usecase.ModelTokenLimits(m.base) }
func (m *sessionBoundModel) Stream(ctx context.Context, request domain.Request) (port.Stream, error) {
	request.Metadata.SessionID = m.sessionID
	return m.base.Stream(ctx, request)
}

type capabilityOverrideModel struct {
	base          port.LanguageModel
	vision        *bool
	tools         *bool
	contextWindow *int
	tokenLimits   *domain.TokenLimits
}

func withVisionCapability(base port.LanguageModel, vision bool) port.LanguageModel {
	return &capabilityOverrideModel{base: base, vision: &vision}
}

func withToolsCapability(base port.LanguageModel, tools bool) port.LanguageModel {
	return &capabilityOverrideModel{base: base, tools: &tools}
}

func withContextWindow(base port.LanguageModel, tokens int) port.LanguageModel {
	return &capabilityOverrideModel{base: base, contextWindow: &tokens}
}

func withTokenLimits(base port.LanguageModel, limits domain.TokenLimits) port.LanguageModel {
	return &capabilityOverrideModel{base: base, tokenLimits: &limits}
}

func (m *capabilityOverrideModel) Provider() string { return m.base.Provider() }
func (m *capabilityOverrideModel) ModelID() string  { return m.base.ModelID() }
func (m *capabilityOverrideModel) Capabilities() domain.ModelCapabilities {
	caps := m.base.Capabilities()
	if m.vision != nil {
		caps.Vision = *m.vision
	}
	if m.tools != nil {
		caps.Tools = *m.tools
	}
	return caps
}
func (m *capabilityOverrideModel) TokenLimits() domain.TokenLimits {
	limits := usecase.ModelTokenLimits(m.base)
	if m.tokenLimits != nil {
		if m.tokenLimits.ContextWindow > 0 {
			limits.ContextWindow = m.tokenLimits.ContextWindow
		}
		if m.tokenLimits.MaxInputTokens > 0 {
			limits.MaxInputTokens = m.tokenLimits.MaxInputTokens
		}
		if m.tokenLimits.MaxOutputTokens > 0 {
			limits.MaxOutputTokens = m.tokenLimits.MaxOutputTokens
		}
	}
	if m.contextWindow != nil && *m.contextWindow > 0 {
		limits.ContextWindow = *m.contextWindow
	}
	return limits
}
func (m *capabilityOverrideModel) ContextWindow() int {
	return m.TokenLimits().ContextWindow
}
func (m *capabilityOverrideModel) Stream(ctx context.Context, request domain.Request) (port.Stream, error) {
	return m.base.Stream(ctx, request)
}

func newSDKOpenAILanguageModel(providerName, baseURL, apiKey, modelID string, opts ...ClientOption) port.LanguageModel {
	cfg := newClientConfig(baseURL, apiKey, modelID)
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	headers := agentHeaders(cfg)
	isOpenCode := IsOpenCodeRoute(providerName, cfg.baseURL)
	freeStreamRecovery := isOpenCode && IsFreeModel(cfg.modelID)
	lowConcurrencyEnabled := cfg.lowConcurrency.Enabled(isOpenCode && IsFreeModel(cfg.modelID))
	retryPolicy := modelRetryPolicy()
	providerMaxRetries := runtimepolicy.ModelRetryMaxRetries
	if freeStreamRecovery {
		// The stream wrapper owns the complete retry budget for free models.
		// Leaving provider retries enabled here would retry the same logical
		// request once in the provider and again in the wrapper.
		providerMaxRetries = 0
	}
	sessionID := strings.TrimSpace(cfg.sessionID)
	if sessionID != "" {
		headers.Set("x-session-affinity", sessionID)
		if isOpenCode {
			headers.Set("x-opencode-session", sessionID)
		}
	}
	if isOpenCode {
		clientName := cfg.clientName
		if clientName == "" {
			clientName = "proton"
		}
		headers.Set("x-opencode-client", clientName)
	}
	provider := sdkopenai.NewProvider(sdkopenai.ProviderOptions{
		ProviderName: strings.ToLower(strings.TrimSpace(providerName)),
		BaseURL:      cfg.baseURL, APIKey: cfg.apiKey, HTTPClient: cfg.httpClient,
		UserAgent: cfg.userAgent, Headers: headers, MaxRetries: providerMaxRetries,
		RetryBackoff: retryPolicy.BaseBackoff, RetryPostFirstGap: retryPolicy.PostFirstRetryGap,
		MaxRetryBackoff: retryPolicy.MaxBackoff, MaxRetryAfter: retryPolicy.MaxRetryAfter,
		RetryDelays: retryPolicy.RetryDelays,
	})
	modelOptions := make([]sdkopenai.ModelOption, 0, 1)
	if cfg.responsesAPI || usesResponsesAPI(cfg.modelID, cfg.baseURL) {
		modelOptions = append(modelOptions, sdkopenai.WithResponsesAPI())
	}
	var model port.LanguageModel = provider.Model(cfg.modelID, modelOptions...)
	if cfg.vision != nil {
		model = withVisionCapability(model, *cfg.vision)
	}
	if cfg.tools != nil {
		model = withToolsCapability(model, *cfg.tools)
	}
	if cfg.tokenLimits != nil {
		model = withTokenLimits(model, *cfg.tokenLimits)
	}
	if cfg.contextWindow != nil {
		model = withContextWindow(model, *cfg.contextWindow)
	}
	model = withSessionID(model, sessionID)
	if lowConcurrencyEnabled {
		model = withLowConcurrencyMode(model, providerName, lowConcurrencyRoute(providerName, cfg.baseURL, cfg.modelID), runtimepolicy.LowConcurrencyMode())
	}
	if freeStreamRecovery {
		model = withStreamRetryPolicyConfig(model, runtimepolicy.ModelRetryMaxRetries, retryPolicy,
			runtimepolicy.OpenCodeFreeFirstEventTimeout,
			runtimepolicy.OpenCodeFreeIdleEventTimeout,
			runtimepolicy.OpenCodeFreeStreamMaxDuration)
	} else {
		model = withReplaySafeRetryConfig(model, runtimepolicy.ModelStreamReplayMaxRetries, retryPolicy)
	}
	return withModelProfile(model, cfg.profile)
}

func newSDKAnthropicLanguageModel(providerName, baseURL, apiKey, modelID string, opts ...ClientOption) port.LanguageModel {
	cfg := newClientConfig(baseURL, apiKey, modelID)
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	sessionID := strings.TrimSpace(cfg.sessionID)
	retryPolicy := modelRetryPolicy()
	headers := agentHeaders(cfg)
	if isOpenCodeZenRoute(providerName, cfg.baseURL) {
		if sessionID != "" {
			headers.Set("x-opencode-session", sessionID)
		}
		clientName := cfg.clientName
		if clientName == "" {
			clientName = "proton"
		}
		headers.Set("x-opencode-client", clientName)
	}
	provider := sdkanthropic.NewProvider(sdkanthropic.ProviderOptions{
		BaseURL: cfg.baseURL, APIKey: cfg.apiKey, HTTPClient: cfg.httpClient, Headers: headers,
		UserAgent: cfg.userAgent, MaxRetries: runtimepolicy.ModelRetryMaxRetries,
		RetryBackoff: retryPolicy.BaseBackoff, RetryPostFirstGap: retryPolicy.PostFirstRetryGap,
		MaxRetryBackoff: retryPolicy.MaxBackoff, MaxRetryAfter: retryPolicy.MaxRetryAfter,
		RetryDelays: retryPolicy.RetryDelays,
	})
	var model port.LanguageModel = provider.Model(cfg.modelID)
	if cfg.vision != nil {
		model = withVisionCapability(model, *cfg.vision)
	}
	if cfg.tools != nil {
		model = withToolsCapability(model, *cfg.tools)
	}
	if cfg.tokenLimits != nil {
		model = withTokenLimits(model, *cfg.tokenLimits)
	}
	if cfg.contextWindow != nil {
		model = withContextWindow(model, *cfg.contextWindow)
	}
	model = withSessionID(model, sessionID)
	if cfg.lowConcurrency.Enabled(false) {
		model = withLowConcurrencyMode(model, providerName, lowConcurrencyRoute(providerName, cfg.baseURL, cfg.modelID), runtimepolicy.LowConcurrencyMode())
	}
	model = withReplaySafeRetryConfig(model, runtimepolicy.ModelStreamReplayMaxRetries, retryPolicy)
	return withModelProfile(model, cfg.profile)
}

func usesResponsesAPI(modelID, baseURL string) bool {
	id := modelIDLeafForTransport(modelID)
	return strings.HasPrefix(id, "muse-spark") || strings.Contains(id, "responses") || strings.HasSuffix(strings.TrimSpace(baseURL), "/responses")
}

func agentHeaders(cfg clientConfig) http.Header {
	headers := make(http.Header)
	if cfg.agentType != "" {
		headers.Set("X-Agent-Type", cfg.agentType.String())
	}
	if version := strings.TrimSpace(buildinfo.Version()); version != "" {
		headers.Set("X-Agent-Version", version)
	}
	if profile := strings.TrimSpace(cfg.agentProfile); profile != "" {
		headers.Set("X-Agent-Profile", profile)
	}
	return headers
}

var (
	errOpenCodeFreeNoOutputTimeout = errors.New("opencode free model produced no output before timeout")
	errOpenCodeFreeIdleTimeout     = errors.New("opencode free model stream became idle before completion")
	errOpenCodeFreeMaxDuration     = errors.New("opencode free model stream exceeded maximum duration")
)

func modelRetryPolicy() domain.RetryPolicy {
	return domain.RetryPolicy{
		BaseBackoff:       runtimepolicy.ModelRetryBackoffStep,
		PostFirstRetryGap: runtimepolicy.ModelRetryPostFirstGap,
		MaxBackoff:        runtimepolicy.ModelRetryMaxBackoff,
		MaxRetryAfter:     runtimepolicy.ModelRetryMaxRetryAfter,
		RetryDelays:       runtimepolicy.ModelRetrySchedule(),
	}
}

type streamRetryProgress uint8

const (
	streamProgressEmpty streamRetryProgress = iota
	streamProgressBufferedTool
	streamProgressCommittedText
)

func (p streamRetryProgress) replaySafe() bool {
	return p != streamProgressCommittedText
}

type emptyStreamRetryModel struct {
	base              port.LanguageModel
	maxRetries        int
	retryPolicy       domain.RetryPolicy
	firstEventTimeout time.Duration
	idleEventTimeout  time.Duration
	maxStreamDuration time.Duration
	// retryOpenFailures reports whether open failures are retried by the
	// wrapper. Free-model recovery owns the complete retry budget
	// (provider retries disabled), so it retries opens. Generic replay-safe
	// recovery keeps provider open retries intact and only retries
	// replay-safe mid-stream prefixes, so it must not double-retry opens.
	retryOpenFailures bool
}

func withEmptyStreamRetry(base port.LanguageModel, maxRetries int, backoff time.Duration) port.LanguageModel {
	policy := modelRetryPolicy()
	policy.BaseBackoff = backoff
	policy.RetryDelays = nil
	return withStreamRetryPolicyConfig(base, maxRetries, policy,
		runtimepolicy.OpenCodeFreeFirstEventTimeout,
		runtimepolicy.OpenCodeFreeIdleEventTimeout,
		runtimepolicy.OpenCodeFreeStreamMaxDuration,
	)
}

func withEmptyStreamRetryPolicy(base port.LanguageModel, maxRetries int, backoff, noOutputTimeout time.Duration) port.LanguageModel {
	return withStreamRetryPolicyConfig(base, maxRetries, streamRetryPolicy(backoff, 0), noOutputTimeout, 0, 0)
}

func withStreamRetryPolicy(base port.LanguageModel, maxRetries int, backoff, firstEventTimeout, idleEventTimeout, maxStreamDuration time.Duration) port.LanguageModel {
	return withStreamRetryPolicyConfig(base, maxRetries, streamRetryPolicy(backoff, 0), firstEventTimeout, idleEventTimeout, maxStreamDuration)
}

func withStreamRetryPolicyAndGap(base port.LanguageModel, maxRetries int, backoff, postFirstRetryGap, firstEventTimeout, idleEventTimeout, maxStreamDuration time.Duration) port.LanguageModel {
	return withStreamRetryPolicyConfig(base, maxRetries, streamRetryPolicy(backoff, postFirstRetryGap), firstEventTimeout, idleEventTimeout, maxStreamDuration)
}

func streamRetryPolicy(backoff, postFirstRetryGap time.Duration) domain.RetryPolicy {
	policy := modelRetryPolicy()
	policy.BaseBackoff = backoff
	policy.PostFirstRetryGap = postFirstRetryGap
	// Test and explicit stream callers that override the backoff use the
	// legacy formula; the production model policy keeps the exact schedule.
	policy.RetryDelays = nil
	return policy
}

func withStreamRetryPolicyConfig(base port.LanguageModel, maxRetries int, policy domain.RetryPolicy, firstEventTimeout, idleEventTimeout, maxStreamDuration time.Duration) port.LanguageModel {
	if base == nil || maxRetries <= 0 {
		return base
	}
	return &emptyStreamRetryModel{
		base: base, maxRetries: maxRetries, retryPolicy: policy,
		firstEventTimeout: firstEventTimeout, idleEventTimeout: idleEventTimeout, maxStreamDuration: maxStreamDuration,
		retryOpenFailures: true,
	}
}

// withReplaySafeRetryConfig wraps a model with replay-safe incomplete-stream
// recovery that never replays committed text. Unlike the free-model wrapper,
// it does not retry stream opens: provider transport already owns the open
// retry budget, so retrying opens here would double-count. Only replay-safe
// mid-stream prefixes and empty finishes consume this budget.
func withReplaySafeRetryConfig(base port.LanguageModel, maxRetries int, policy domain.RetryPolicy) port.LanguageModel {
	if base == nil || maxRetries <= 0 {
		return base
	}
	return &emptyStreamRetryModel{
		base: base, maxRetries: maxRetries, retryPolicy: policy,
		retryOpenFailures: false,
	}
}

func (m *emptyStreamRetryModel) Provider() string                       { return m.base.Provider() }
func (m *emptyStreamRetryModel) ModelID() string                        { return m.base.ModelID() }
func (m *emptyStreamRetryModel) Capabilities() domain.ModelCapabilities { return m.base.Capabilities() }
func (m *emptyStreamRetryModel) ContextWindow() int                     { return usecase.ModelContextWindow(m.base) }
func (m *emptyStreamRetryModel) TokenLimits() domain.TokenLimits {
	return usecase.ModelTokenLimits(m.base)
}

func (m *emptyStreamRetryModel) Stream(ctx context.Context, request domain.Request) (port.Stream, error) {
	retry := &emptyStreamRetry{
		base: m.base, request: request, parentCtx: ctx,
		maxRetries: m.maxRetries, retryPolicy: m.retryPolicy,
		firstEventTimeout: m.firstEventTimeout, idleEventTimeout: m.idleEventTimeout, maxStreamDuration: m.maxStreamDuration,
		retryOpenFailures: m.retryOpenFailures,
	}
	if !m.retryOpenFailures {
		if err := retry.openAttempt(); err != nil {
			return nil, err
		}
		return retry, nil
	}
	if err := retry.openWithRetry(ctx); err != nil {
		return nil, err
	}
	return retry, nil
}

type emptyStreamRetry struct {
	base              port.LanguageModel
	request           domain.Request
	parentCtx         context.Context
	attemptCtx        context.Context
	cancelAttempt     context.CancelCauseFunc
	firstEventTimer   *time.Timer
	idleEventTimer    *time.Timer
	maxStreamTimer    *time.Timer
	stream            port.Stream
	maxRetries        int
	retryPolicy       domain.RetryPolicy
	firstEventTimeout time.Duration
	idleEventTimeout  time.Duration
	maxStreamDuration time.Duration
	retries           int
	progress          streamRetryProgress
	pending           []domain.Event
	queue             []domain.Event
	retryOpenFailures bool
}

func (s *emptyStreamRetry) Next(ctx context.Context) (domain.Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return domain.Event{}, err
		}
		if len(s.queue) > 0 {
			event := s.queue[0]
			s.queue = s.queue[1:]
			return event, nil
		}

		event, err := s.stream.Next(s.attemptCtx)
		if err != nil {
			if timeoutCause := s.streamTimeoutCause(); timeoutCause != nil {
				if s.progress.replaySafe() && s.retries < s.maxRetries && s.canRetry(timeoutCause, s.retries+1) {
					if retryErr := s.retry(ctx, streamTimeoutReason(timeoutCause), timeoutCause); retryErr != nil {
						return domain.Event{}, retryErr
					}
					if retryErr := s.reopenAfterRetry(ctx); retryErr != nil {
						return domain.Event{}, fmt.Errorf("retry empty model stream: %w", retryErr)
					}
					continue
				}
				s.stopAttemptTimers()
				return domain.Event{}, fmt.Errorf("%w: %w", domain.ErrIncompleteStream, timeoutCause)
			}
			if reason, retryable := retryableStreamError(err); retryable && s.progress.replaySafe() && s.retries < s.maxRetries && s.canRetry(err, s.retries+1) {
				if retryErr := s.retry(ctx, reason, err); retryErr != nil {
					return domain.Event{}, retryErr
				}
				if retryErr := s.reopenAfterRetry(ctx); retryErr != nil {
					return domain.Event{}, fmt.Errorf("retry empty model stream: %w", retryErr)
				}
				continue
			}
			s.stopAttemptTimers()
			return domain.Event{}, err
		}
		s.observeStreamActivity()

		switch streamEventProgress(event) {
		case streamProgressCommittedText:
			if s.progress != streamProgressCommittedText {
				s.progress = streamProgressCommittedText
				if len(s.pending) > 0 {
					s.queue = append(s.queue, s.pending...)
					s.pending = nil
					s.queue = append(s.queue, event)
					continue
				}
			}
		case streamProgressBufferedTool:
			if s.progress != streamProgressCommittedText {
				s.progress = streamProgressBufferedTool
				s.pending = append(s.pending, event)
				continue
			}
		default:
			if s.progress != streamProgressCommittedText && event.Kind != domain.EventFinish {
				s.pending = append(s.pending, event)
				continue
			}
		}

		if event.Kind == domain.EventFinish {
			s.stopAttemptTimers()
			switch s.progress {
			case streamProgressEmpty:
				if event.FinishReason == domain.FinishStop && s.retries < s.maxRetries && s.canRetry(nil, s.retries+1) {
					s.pending = nil
					if retryErr := s.retry(ctx, "empty_finish", nil); retryErr != nil {
						return domain.Event{}, retryErr
					}
					if retryErr := s.reopenAfterRetry(ctx); retryErr != nil {
						return domain.Event{}, fmt.Errorf("retry empty model stream: %w", retryErr)
					}
					continue
				}
				if len(s.pending) > 0 {
					s.queue = append(s.queue, s.pending...)
					s.pending = nil
					s.queue = append(s.queue, event)
					continue
				}
			case streamProgressBufferedTool:
				s.queue = append(s.queue, s.pending...)
				s.pending = nil
				s.queue = append(s.queue, event)
				continue
			}
		}
		return event, nil
	}
}

func (s *emptyStreamRetry) retry(ctx context.Context, reason string, cause error) error {
	retryIndex := s.retries + 1
	decision := s.retryDecision(cause, retryIndex)
	if !decision.Retry {
		return cause
	}
	_ = s.closeAttempt()
	s.progress = streamProgressEmpty
	s.pending = nil
	s.queue = nil
	s.retries = retryIndex
	if err := s.scheduleRetry(ctx, reason, decision); err != nil {
		return err
	}
	return nil
}

func (s *emptyStreamRetry) openWithRetry(ctx context.Context) error {
	for {
		err := s.openAttempt()
		if err == nil {
			return nil
		}
		reason, retryable := retryableStreamError(err)
		if !retryable || s.retries >= s.maxRetries || !s.canRetry(err, s.retries+1) {
			return err
		}
		if retryErr := s.retry(ctx, reason, err); retryErr != nil {
			return retryErr
		}
	}
}

// reopenAfterRetry opens the next attempt after a mid-stream retry. Wrappers
// that own the open budget (free-model recovery) reuse openWithRetry so a
// retryable open failure consumes the same budget. Generic replay-safe
// recovery leaves open retries to provider transport and opens exactly once
// so one logical retry never double-counts.
func (s *emptyStreamRetry) reopenAfterRetry(ctx context.Context) error {
	if s.retryOpenFailures {
		return s.openWithRetry(ctx)
	}
	return s.openAttempt()
}

func (s *emptyStreamRetry) canRetry(cause error, retryIndex int) bool {
	return s.retryDecision(cause, retryIndex).Retry
}

func (s *emptyStreamRetry) retryDecision(cause error, retryIndex int) domain.RetryDecision {
	var providerErr *domain.ProviderError
	if errors.As(cause, &providerErr) && providerErr != nil {
		decision := domain.DecideRetry(cause, retryIndex, s.retryPolicy)
		// A zero backoff is useful for tests and explicit callers. Preserve an
		// explicit provider Retry-After, but do not replace disabled local
		// backoff with the SDK fallback.
		if decision.Retry && s.retryPolicy.BaseBackoff <= 0 && (providerErr.RateLimit == nil || providerErr.RateLimit.RetryAfter <= 0) {
			decision.Delay = 0
		}
		return decision
	}
	if s.retryPolicy.BaseBackoff <= 0 {
		return domain.RetryDecision{Retry: true}
	}
	return domain.RetryDecision{Retry: true, Delay: domain.RetryDelay(retryIndex, s.retryPolicy)}
}

func (s *emptyStreamRetry) scheduleRetry(ctx context.Context, reason string, decision domain.RetryDecision) error {
	delay := decision.Delay
	slog.DebugContext(ctx, "model stream is replay-safe; retrying",
		"provider", s.base.Provider(), "model", s.base.ModelID(),
		"reason", reason, "retry", s.retries, "max_retries", s.maxRetries,
		"delay_ms", delay.Milliseconds(),
	)
	domain.ObserveRetry(ctx, domain.RetryEvent{
		Provider: s.base.Provider(), ModelID: s.base.ModelID(), Reason: reason,
		Attempt: s.retries, MaxRetries: s.maxRetries, Delay: delay,
	})
	if err := domain.WaitForRetry(ctx, delay); err != nil {
		return fmt.Errorf("wait to retry empty model stream: %w", err)
	}
	return nil
}

func retryableStreamError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if isStreamTimeoutCause(err) {
		return streamTimeoutReason(err), true
	}
	if errors.Is(err, domain.ErrIncompleteStream) {
		return "incomplete_stream", true
	}
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil || !providerErr.Retryable {
		return "", false
	}
	switch providerErr.Kind {
	case domain.ErrorTransport:
		return "provider_transport", true
	case domain.ErrorOverloaded:
		return "provider_overloaded", true
	case domain.ErrorRateLimit:
		return "provider_rate_limit", true
	default:
		return "provider_stream_error", true
	}
}

func (s *emptyStreamRetry) openAttempt() error {
	attemptCtx, cancel := context.WithCancelCause(s.parentCtx)
	var firstTimer, maxTimer *time.Timer
	if s.firstEventTimeout > 0 {
		firstTimer = time.AfterFunc(s.firstEventTimeout, func() {
			cancel(errOpenCodeFreeNoOutputTimeout)
		})
	}
	if s.maxStreamDuration > 0 {
		maxTimer = time.AfterFunc(s.maxStreamDuration, func() {
			cancel(errOpenCodeFreeMaxDuration)
		})
	}
	stream, err := s.base.Stream(attemptCtx, s.request)
	if err != nil {
		if firstTimer != nil {
			firstTimer.Stop()
		}
		if maxTimer != nil {
			maxTimer.Stop()
		}
		cause := context.Cause(attemptCtx)
		cancel(nil)
		if isStreamTimeoutCause(cause) {
			return fmt.Errorf("%w: %w", domain.ErrIncompleteStream, cause)
		}
		return err
	}
	if stream == nil {
		if firstTimer != nil {
			firstTimer.Stop()
		}
		if maxTimer != nil {
			maxTimer.Stop()
		}
		cancel(nil)
		return fmt.Errorf("open model stream: nil stream")
	}
	s.attemptCtx = attemptCtx
	s.cancelAttempt = cancel
	s.firstEventTimer = firstTimer
	s.maxStreamTimer = maxTimer
	s.stream = stream
	return nil
}

func (s *emptyStreamRetry) observeStreamActivity() {
	if s.firstEventTimer != nil {
		s.firstEventTimer.Stop()
		s.firstEventTimer = nil
	}
	if s.idleEventTimeout <= 0 || s.cancelAttempt == nil {
		return
	}
	if s.idleEventTimer != nil {
		s.idleEventTimer.Stop()
	}
	cancel := s.cancelAttempt
	s.idleEventTimer = time.AfterFunc(s.idleEventTimeout, func() {
		cancel(errOpenCodeFreeIdleTimeout)
	})
}

func (s *emptyStreamRetry) streamTimeoutCause() error {
	if s.attemptCtx == nil {
		return nil
	}
	cause := context.Cause(s.attemptCtx)
	if isStreamTimeoutCause(cause) {
		return cause
	}
	return nil
}

func isStreamTimeoutCause(err error) bool {
	return errors.Is(err, errOpenCodeFreeNoOutputTimeout) ||
		errors.Is(err, errOpenCodeFreeIdleTimeout) ||
		errors.Is(err, errOpenCodeFreeMaxDuration)
}

func streamTimeoutReason(err error) string {
	switch {
	case errors.Is(err, errOpenCodeFreeNoOutputTimeout):
		return "first_event_timeout"
	case errors.Is(err, errOpenCodeFreeIdleTimeout):
		return "idle_event_timeout"
	case errors.Is(err, errOpenCodeFreeMaxDuration):
		return "max_stream_duration"
	default:
		return "stream_timeout"
	}
}

func (s *emptyStreamRetry) stopAttemptTimers() {
	for _, timer := range []*time.Timer{s.firstEventTimer, s.idleEventTimer, s.maxStreamTimer} {
		if timer != nil {
			timer.Stop()
		}
	}
	s.firstEventTimer = nil
	s.idleEventTimer = nil
	s.maxStreamTimer = nil
}

func (s *emptyStreamRetry) closeAttempt() error {
	s.stopAttemptTimers()
	if s.cancelAttempt != nil {
		s.cancelAttempt(nil)
		s.cancelAttempt = nil
	}
	if s.stream == nil {
		return nil
	}
	err := s.stream.Close()
	s.stream = nil
	s.attemptCtx = nil
	return err
}

func (s *emptyStreamRetry) Close() error {
	return s.closeAttempt()
}

func streamEventProgress(event domain.Event) streamRetryProgress {
	switch event.Kind {
	case domain.EventTextDelta:
		if event.Text != "" {
			return streamProgressCommittedText
		}
	case domain.EventToolCallStart, domain.EventToolCallDelta, domain.EventToolCallEnd, domain.EventToolCall:
		return streamProgressBufferedTool
	}
	return streamProgressEmpty
}
