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
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
	sdkanthropic "github.com/phongsathornpt/protonman/proton-sdk/provider/anthropic"
	sdkopenai "github.com/phongsathornpt/protonman/proton-sdk/provider/openai"
)

type sessionBoundModel struct {
	base      sdk.LanguageModel
	sessionID string
}

func withSessionID(base sdk.LanguageModel, sessionID string) sdk.LanguageModel {
	sessionID = strings.TrimSpace(sessionID)
	if base == nil || sessionID == "" {
		return base
	}
	return &sessionBoundModel{base: base, sessionID: sessionID}
}

func (m *sessionBoundModel) Provider() string                    { return m.base.Provider() }
func (m *sessionBoundModel) ModelID() string                     { return m.base.ModelID() }
func (m *sessionBoundModel) Capabilities() sdk.ModelCapabilities { return m.base.Capabilities() }
func (m *sessionBoundModel) ContextWindow() int                  { return sdk.ModelContextWindow(m.base) }
func (m *sessionBoundModel) TokenLimits() sdk.TokenLimits        { return sdk.ModelTokenLimits(m.base) }
func (m *sessionBoundModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	request.Metadata.SessionID = m.sessionID
	return m.base.Stream(ctx, request)
}

type capabilityOverrideModel struct {
	base          sdk.LanguageModel
	vision        *bool
	tools         *bool
	contextWindow *int
	tokenLimits   *sdk.TokenLimits
}

func withVisionCapability(base sdk.LanguageModel, vision bool) sdk.LanguageModel {
	return &capabilityOverrideModel{base: base, vision: &vision}
}

func withToolsCapability(base sdk.LanguageModel, tools bool) sdk.LanguageModel {
	return &capabilityOverrideModel{base: base, tools: &tools}
}

func withContextWindow(base sdk.LanguageModel, tokens int) sdk.LanguageModel {
	return &capabilityOverrideModel{base: base, contextWindow: &tokens}
}

func withTokenLimits(base sdk.LanguageModel, limits sdk.TokenLimits) sdk.LanguageModel {
	return &capabilityOverrideModel{base: base, tokenLimits: &limits}
}

func (m *capabilityOverrideModel) Provider() string { return m.base.Provider() }
func (m *capabilityOverrideModel) ModelID() string  { return m.base.ModelID() }
func (m *capabilityOverrideModel) Capabilities() sdk.ModelCapabilities {
	caps := m.base.Capabilities()
	if m.vision != nil {
		caps.Vision = *m.vision
	}
	if m.tools != nil {
		caps.Tools = *m.tools
	}
	return caps
}
func (m *capabilityOverrideModel) TokenLimits() sdk.TokenLimits {
	limits := sdk.ModelTokenLimits(m.base)
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
func (m *capabilityOverrideModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	return m.base.Stream(ctx, request)
}

func newSDKOpenAILanguageModel(providerName, baseURL, apiKey, modelID string, opts ...ClientOption) sdk.LanguageModel {
	cfg := newClientConfig(baseURL, apiKey, modelID)
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	headers := agentHeaders(cfg)
	isOpenCode := IsProvider(DefaultOpenCodeName, providerName, cfg.baseURL)
	freeStreamRecovery := isOpenCode && IsFreeModel(cfg.modelID)
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
	})
	modelOptions := make([]sdkopenai.ModelOption, 0, 1)
	if usesResponsesAPI(cfg.modelID, cfg.baseURL) {
		modelOptions = append(modelOptions, sdkopenai.WithResponsesAPI())
	}
	var model sdk.LanguageModel = provider.Model(cfg.modelID, modelOptions...)
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
	if freeStreamRecovery {
		model = withStreamRetryPolicyConfig(model, runtimepolicy.ModelRetryMaxRetries, retryPolicy,
			runtimepolicy.OpenCodeFreeFirstEventTimeout,
			runtimepolicy.OpenCodeFreeIdleEventTimeout,
			runtimepolicy.OpenCodeFreeStreamMaxDuration)
	}
	return withModelProfile(model, cfg.profile)
}

func newSDKAnthropicLanguageModel(baseURL, apiKey, modelID string, opts ...ClientOption) sdk.LanguageModel {
	cfg := newClientConfig(baseURL, apiKey, modelID)
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	sessionID := strings.TrimSpace(cfg.sessionID)
	retryPolicy := modelRetryPolicy()
	provider := sdkanthropic.NewProvider(sdkanthropic.ProviderOptions{
		BaseURL: cfg.baseURL, APIKey: cfg.apiKey, HTTPClient: cfg.httpClient, Headers: agentHeaders(cfg),
		UserAgent: cfg.userAgent, MaxRetries: runtimepolicy.ModelRetryMaxRetries,
		RetryBackoff: retryPolicy.BaseBackoff, RetryPostFirstGap: retryPolicy.PostFirstRetryGap,
		MaxRetryBackoff: retryPolicy.MaxBackoff, MaxRetryAfter: retryPolicy.MaxRetryAfter,
	})
	var model sdk.LanguageModel = provider.Model(cfg.modelID)
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
	return withModelProfile(withSessionID(model, sessionID), cfg.profile)
}

func usesResponsesAPI(modelID, baseURL string) bool {
	id := strings.ToLower(strings.TrimSpace(modelID))
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

func modelRetryPolicy() sdk.RetryPolicy {
	return sdk.RetryPolicy{
		BaseBackoff:       runtimepolicy.ModelRetryBackoffStep,
		PostFirstRetryGap: runtimepolicy.ModelRetryPostFirstGap,
		MaxBackoff:        runtimepolicy.ModelRetryMaxBackoff,
		MaxRetryAfter:     runtimepolicy.ModelRetryMaxRetryAfter,
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
	base              sdk.LanguageModel
	maxRetries        int
	retryPolicy       sdk.RetryPolicy
	firstEventTimeout time.Duration
	idleEventTimeout  time.Duration
	maxStreamDuration time.Duration
}

func withEmptyStreamRetry(base sdk.LanguageModel, maxRetries int, backoff time.Duration) sdk.LanguageModel {
	policy := modelRetryPolicy()
	policy.BaseBackoff = backoff
	return withStreamRetryPolicyConfig(base, maxRetries, policy,
		runtimepolicy.OpenCodeFreeFirstEventTimeout,
		runtimepolicy.OpenCodeFreeIdleEventTimeout,
		runtimepolicy.OpenCodeFreeStreamMaxDuration,
	)
}

func withEmptyStreamRetryPolicy(base sdk.LanguageModel, maxRetries int, backoff, noOutputTimeout time.Duration) sdk.LanguageModel {
	return withStreamRetryPolicyConfig(base, maxRetries, streamRetryPolicy(backoff, 0), noOutputTimeout, 0, 0)
}

func withStreamRetryPolicy(base sdk.LanguageModel, maxRetries int, backoff, firstEventTimeout, idleEventTimeout, maxStreamDuration time.Duration) sdk.LanguageModel {
	return withStreamRetryPolicyConfig(base, maxRetries, streamRetryPolicy(backoff, 0), firstEventTimeout, idleEventTimeout, maxStreamDuration)
}

func withStreamRetryPolicyAndGap(base sdk.LanguageModel, maxRetries int, backoff, postFirstRetryGap, firstEventTimeout, idleEventTimeout, maxStreamDuration time.Duration) sdk.LanguageModel {
	return withStreamRetryPolicyConfig(base, maxRetries, streamRetryPolicy(backoff, postFirstRetryGap), firstEventTimeout, idleEventTimeout, maxStreamDuration)
}

func streamRetryPolicy(backoff, postFirstRetryGap time.Duration) sdk.RetryPolicy {
	policy := modelRetryPolicy()
	policy.BaseBackoff = backoff
	policy.PostFirstRetryGap = postFirstRetryGap
	return policy
}

func withStreamRetryPolicyConfig(base sdk.LanguageModel, maxRetries int, policy sdk.RetryPolicy, firstEventTimeout, idleEventTimeout, maxStreamDuration time.Duration) sdk.LanguageModel {
	if base == nil || maxRetries <= 0 {
		return base
	}
	return &emptyStreamRetryModel{
		base: base, maxRetries: maxRetries, retryPolicy: policy,
		firstEventTimeout: firstEventTimeout, idleEventTimeout: idleEventTimeout, maxStreamDuration: maxStreamDuration,
	}
}

func (m *emptyStreamRetryModel) Provider() string                    { return m.base.Provider() }
func (m *emptyStreamRetryModel) ModelID() string                     { return m.base.ModelID() }
func (m *emptyStreamRetryModel) Capabilities() sdk.ModelCapabilities { return m.base.Capabilities() }
func (m *emptyStreamRetryModel) ContextWindow() int                  { return sdk.ModelContextWindow(m.base) }
func (m *emptyStreamRetryModel) TokenLimits() sdk.TokenLimits        { return sdk.ModelTokenLimits(m.base) }

func (m *emptyStreamRetryModel) Stream(ctx context.Context, request sdk.Request) (sdk.Stream, error) {
	retry := &emptyStreamRetry{
		base: m.base, request: request, parentCtx: ctx,
		maxRetries: m.maxRetries, retryPolicy: m.retryPolicy,
		firstEventTimeout: m.firstEventTimeout, idleEventTimeout: m.idleEventTimeout, maxStreamDuration: m.maxStreamDuration,
	}
	if err := retry.openWithRetry(ctx); err != nil {
		return nil, err
	}
	return retry, nil
}

type emptyStreamRetry struct {
	base              sdk.LanguageModel
	request           sdk.Request
	parentCtx         context.Context
	attemptCtx        context.Context
	cancelAttempt     context.CancelCauseFunc
	firstEventTimer   *time.Timer
	idleEventTimer    *time.Timer
	maxStreamTimer    *time.Timer
	stream            sdk.Stream
	maxRetries        int
	retryPolicy       sdk.RetryPolicy
	firstEventTimeout time.Duration
	idleEventTimeout  time.Duration
	maxStreamDuration time.Duration
	retries           int
	progress          streamRetryProgress
	pending           []sdk.Event
	queue             []sdk.Event
}

func (s *emptyStreamRetry) Next(ctx context.Context) (sdk.Event, error) {
	for {
		if err := ctx.Err(); err != nil {
			return sdk.Event{}, err
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
						return sdk.Event{}, retryErr
					}
					if retryErr := s.openWithRetry(ctx); retryErr != nil {
						return sdk.Event{}, fmt.Errorf("retry empty model stream: %w", retryErr)
					}
					continue
				}
				s.stopAttemptTimers()
				return sdk.Event{}, fmt.Errorf("%w: %w", sdk.ErrIncompleteStream, timeoutCause)
			}
			if reason, retryable := retryableStreamError(err); retryable && s.progress.replaySafe() && s.retries < s.maxRetries && s.canRetry(err, s.retries+1) {
				if retryErr := s.retry(ctx, reason, err); retryErr != nil {
					return sdk.Event{}, retryErr
				}
				if retryErr := s.openWithRetry(ctx); retryErr != nil {
					return sdk.Event{}, fmt.Errorf("retry empty model stream: %w", retryErr)
				}
				continue
			}
			s.stopAttemptTimers()
			return sdk.Event{}, err
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
			if s.progress != streamProgressCommittedText && event.Kind != sdk.EventFinish {
				s.pending = append(s.pending, event)
				continue
			}
		}

		if event.Kind == sdk.EventFinish {
			s.stopAttemptTimers()
			switch s.progress {
			case streamProgressEmpty:
				if event.FinishReason == sdk.FinishStop && s.retries < s.maxRetries && s.canRetry(nil, s.retries+1) {
					s.pending = nil
					if retryErr := s.retry(ctx, "empty_finish", nil); retryErr != nil {
						return sdk.Event{}, retryErr
					}
					if retryErr := s.openWithRetry(ctx); retryErr != nil {
						return sdk.Event{}, fmt.Errorf("retry empty model stream: %w", retryErr)
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

func (s *emptyStreamRetry) canRetry(cause error, retryIndex int) bool {
	return s.retryDecision(cause, retryIndex).Retry
}

func (s *emptyStreamRetry) retryDecision(cause error, retryIndex int) sdk.RetryDecision {
	var providerErr *sdk.ProviderError
	if errors.As(cause, &providerErr) && providerErr != nil {
		decision := sdk.DecideRetry(cause, retryIndex, s.retryPolicy)
		// A zero backoff is useful for tests and explicit callers. Preserve an
		// explicit provider Retry-After, but do not replace disabled local
		// backoff with the SDK fallback.
		if decision.Retry && s.retryPolicy.BaseBackoff <= 0 && (providerErr.RateLimit == nil || providerErr.RateLimit.RetryAfter <= 0) {
			decision.Delay = 0
		}
		return decision
	}
	if s.retryPolicy.BaseBackoff <= 0 {
		return sdk.RetryDecision{Retry: true}
	}
	return sdk.RetryDecision{Retry: true, Delay: sdk.RetryDelay(retryIndex, s.retryPolicy)}
}

func (s *emptyStreamRetry) scheduleRetry(ctx context.Context, reason string, decision sdk.RetryDecision) error {
	delay := decision.Delay
	slog.DebugContext(ctx, "opencode free model stream is replay-safe; retrying",
		"provider", s.base.Provider(), "model", s.base.ModelID(),
		"reason", reason, "retry", s.retries, "max_retries", s.maxRetries,
		"delay_ms", delay.Milliseconds(),
	)
	sdk.ObserveRetry(ctx, sdk.RetryEvent{
		Provider: s.base.Provider(), ModelID: s.base.ModelID(), Reason: reason,
		Attempt: s.retries, MaxRetries: s.maxRetries, Delay: delay,
	})
	if err := sdk.WaitForRetry(ctx, delay); err != nil {
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
	if errors.Is(err, sdk.ErrIncompleteStream) {
		return "incomplete_stream", true
	}
	var providerErr *sdk.ProviderError
	if !errors.As(err, &providerErr) || providerErr == nil || !providerErr.Retryable {
		return "", false
	}
	switch providerErr.Kind {
	case sdk.ErrorTransport:
		return "provider_transport", true
	case sdk.ErrorOverloaded:
		return "provider_overloaded", true
	case sdk.ErrorRateLimit:
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
			return fmt.Errorf("%w: %w", sdk.ErrIncompleteStream, cause)
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

func streamEventProgress(event sdk.Event) streamRetryProgress {
	switch event.Kind {
	case sdk.EventTextDelta:
		if event.Text != "" {
			return streamProgressCommittedText
		}
	case sdk.EventToolCallStart, sdk.EventToolCallDelta, sdk.EventToolCallEnd, sdk.EventToolCall:
		return streamProgressBufferedTool
	}
	return streamProgressEmpty
}
