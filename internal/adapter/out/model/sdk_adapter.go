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
		UserAgent: cfg.userAgent, Headers: headers, MaxRetries: 2, RetryBackoff: runtimepolicy.ModelRetryBackoffStep,
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
	if isOpenCode && IsFreeModel(cfg.modelID) {
		model = withEmptyStreamRetry(model, openCodeFreeEmptyStreamMaxRetries, runtimepolicy.ModelRetryBackoffStep)
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
	provider := sdkanthropic.NewProvider(sdkanthropic.ProviderOptions{
		BaseURL: cfg.baseURL, APIKey: cfg.apiKey, HTTPClient: cfg.httpClient, Headers: agentHeaders(cfg),
		UserAgent: cfg.userAgent, MaxRetries: 2, RetryBackoff: runtimepolicy.ModelRetryBackoffStep,
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

const (
	openCodeFreeEmptyStreamMaxRetries = 2
	openCodeFreeNoOutputTimeout       = 30 * time.Second
)

var errOpenCodeFreeNoOutputTimeout = errors.New("opencode free model produced no output before timeout")

type emptyStreamRetryModel struct {
	base            sdk.LanguageModel
	maxRetries      int
	backoff         time.Duration
	noOutputTimeout time.Duration
}

func withEmptyStreamRetry(base sdk.LanguageModel, maxRetries int, backoff time.Duration) sdk.LanguageModel {
	return withEmptyStreamRetryPolicy(base, maxRetries, backoff, openCodeFreeNoOutputTimeout)
}

func withEmptyStreamRetryPolicy(base sdk.LanguageModel, maxRetries int, backoff, noOutputTimeout time.Duration) sdk.LanguageModel {
	if base == nil || maxRetries <= 0 {
		return base
	}
	return &emptyStreamRetryModel{
		base: base, maxRetries: maxRetries, backoff: backoff, noOutputTimeout: noOutputTimeout,
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
		maxRetries: m.maxRetries, backoff: m.backoff, noOutputTimeout: m.noOutputTimeout,
	}
	if err := retry.openAttempt(); err != nil {
		return nil, err
	}
	return retry, nil
}

type emptyStreamRetry struct {
	base            sdk.LanguageModel
	request         sdk.Request
	parentCtx       context.Context
	attemptCtx      context.Context
	cancelAttempt   context.CancelCauseFunc
	noOutputTimer   *time.Timer
	stream          sdk.Stream
	maxRetries      int
	backoff         time.Duration
	noOutputTimeout time.Duration
	retries         int
	observedOutput  bool
	pending         []sdk.Event
	queue           []sdk.Event
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
			if s.noOutputTimedOut() && !s.observedOutput {
				if s.retries < s.maxRetries {
					if retryErr := s.retry(ctx, "no_output_timeout"); retryErr != nil {
						return sdk.Event{}, retryErr
					}
					continue
				}
				s.stopNoOutputTimer()
				return sdk.Event{}, fmt.Errorf("%w: %v", sdk.ErrIncompleteStream, errOpenCodeFreeNoOutputTimeout)
			}
			if errors.Is(err, sdk.ErrIncompleteStream) && !s.observedOutput && s.retries < s.maxRetries {
				if retryErr := s.retry(ctx, "incomplete_stream"); retryErr != nil {
					return sdk.Event{}, retryErr
				}
				continue
			}
			s.stopNoOutputTimer()
			return sdk.Event{}, err
		}

		if !s.observedOutput && !streamEventHasOutput(event) && event.Kind != sdk.EventFinish {
			s.pending = append(s.pending, event)
			continue
		}
		if streamEventHasOutput(event) {
			s.observedOutput = true
			s.stopNoOutputTimer()
			if len(s.pending) > 0 {
				s.queue = append(s.queue, s.pending...)
				s.pending = nil
				s.queue = append(s.queue, event)
				continue
			}
		}
		if event.Kind == sdk.EventFinish {
			s.stopNoOutputTimer()
			if !s.observedOutput {
				if event.FinishReason == sdk.FinishStop && s.retries < s.maxRetries {
					s.pending = nil
					if retryErr := s.retry(ctx, "empty_finish"); retryErr != nil {
						return sdk.Event{}, retryErr
					}
					continue
				}
				if len(s.pending) > 0 {
					s.queue = append(s.queue, s.pending...)
					s.pending = nil
					s.queue = append(s.queue, event)
					continue
				}
			}
		}
		return event, nil
	}
}

func (s *emptyStreamRetry) retry(ctx context.Context, reason string) error {
	_ = s.closeAttempt()
	s.retries++
	delay := time.Duration(s.retries) * s.backoff
	slog.WarnContext(ctx, "opencode free model returned no output; retrying stream",
		"provider", s.base.Provider(), "model", s.base.ModelID(),
		"reason", reason, "retry", s.retries, "max_retries", s.maxRetries,
		"delay_ms", delay.Milliseconds(),
	)
	if err := waitForEmptyStreamRetry(ctx, delay); err != nil {
		return fmt.Errorf("wait to retry empty model stream: %w", err)
	}
	if err := s.openAttempt(); err != nil {
		return fmt.Errorf("retry empty model stream: %w", err)
	}
	s.observedOutput = false
	s.pending = nil
	s.queue = nil
	return nil
}

func (s *emptyStreamRetry) openAttempt() error {
	attemptCtx, cancel := context.WithCancelCause(s.parentCtx)
	var timer *time.Timer
	if s.noOutputTimeout > 0 {
		timer = time.AfterFunc(s.noOutputTimeout, func() {
			cancel(errOpenCodeFreeNoOutputTimeout)
		})
	}
	stream, err := s.base.Stream(attemptCtx, s.request)
	if err != nil {
		if timer != nil {
			timer.Stop()
		}
		cause := context.Cause(attemptCtx)
		cancel(nil)
		if errors.Is(cause, errOpenCodeFreeNoOutputTimeout) {
			return fmt.Errorf("%w: %v", sdk.ErrIncompleteStream, errOpenCodeFreeNoOutputTimeout)
		}
		return err
	}
	if stream == nil {
		if timer != nil {
			timer.Stop()
		}
		cancel(nil)
		return fmt.Errorf("open model stream: nil stream")
	}
	s.attemptCtx = attemptCtx
	s.cancelAttempt = cancel
	s.noOutputTimer = timer
	s.stream = stream
	return nil
}

func (s *emptyStreamRetry) noOutputTimedOut() bool {
	return s.attemptCtx != nil && errors.Is(context.Cause(s.attemptCtx), errOpenCodeFreeNoOutputTimeout)
}

func (s *emptyStreamRetry) stopNoOutputTimer() {
	if s.noOutputTimer != nil {
		s.noOutputTimer.Stop()
		s.noOutputTimer = nil
	}
}

func (s *emptyStreamRetry) closeAttempt() error {
	s.stopNoOutputTimer()
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

func streamEventHasOutput(event sdk.Event) bool {
	switch event.Kind {
	case sdk.EventTextDelta:
		return event.Text != ""
	case sdk.EventToolCallStart, sdk.EventToolCallDelta, sdk.EventToolCallEnd, sdk.EventToolCall:
		return true
	default:
		return false
	}
}

func waitForEmptyStreamRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
