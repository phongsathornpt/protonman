package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"weak"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

const (
	methodSessionRuntime           = "protonman/session/runtime"
	methodSessionSetModel          = "protonman/session/set_model"
	methodSessionSetReasoning      = "protonman/session/set_reasoning"
	methodSessionSetLowConcurrency = "protonman/session/set_low_concurrency"
)

// SessionRuntimeSettings is the typed, session-local runtime selection exposed
// to native Protonman clients. These values affect future turns only.
type SessionRuntimeSettings struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Reasoning      string `json:"reasoning"`
	LowConcurrency string `json:"lowConcurrency"`
}

// SessionRuntimeBuilder rebuilds a primary session conversation when model or
// transport policy changes. The ACP adapter owns selection, while composition
// owns concrete provider construction.
type SessionRuntimeBuilder func(
	context.Context,
	string,
	string,
	*toolcall.Service,
	app.Agents,
	SessionRuntimeSettings,
) (app.Conversation, error)

type sessionRuntimeControl struct {
	defaults SessionRuntimeSettings
	build    SessionRuntimeBuilder
}

var sessionRuntimeControls sync.Map   // map[*Server]sessionRuntimeControl
var sessionRuntimeSelections sync.Map // map[weak.Pointer[Session]]SessionRuntimeSettings

// WithSessionRuntimeControls enables typed model/reasoning/low-concurrency
// methods without coupling ACP to concrete provider configuration adapters.
func WithSessionRuntimeControls(defaults SessionRuntimeSettings, build SessionRuntimeBuilder) Option {
	defaults = normalizeSessionRuntime(defaults)
	return func(server *Server) {
		sessionRuntimeControls.Store(server, sessionRuntimeControl{defaults: defaults, build: build})
	}
}

type ProtonmanSessionRuntimeParams struct {
	SessionID string `json:"sessionId"`
}

type ProtonmanSessionRuntimeResult struct {
	SessionID string `json:"sessionId"`
	SessionRuntimeSettings
}

type ProtonmanSessionSetModelParams struct {
	SessionID string `json:"sessionId"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
}

type ProtonmanSessionSetReasoningParams struct {
	SessionID string `json:"sessionId"`
	Reasoning string `json:"reasoning"`
}

type ProtonmanSessionSetLowConcurrencyParams struct {
	SessionID      string `json:"sessionId"`
	LowConcurrency string `json:"lowConcurrency"`
}

func (s *Server) dispatchSessionRuntime(ctx context.Context, request RPCRequest) (any, bool, error) {
	switch request.Method {
	case methodSessionRuntime:
		var params ProtonmanSessionRuntimeParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, true, fmt.Errorf("decode %s: %w", methodSessionRuntime, err)
		}
		sess, err := s.runtimeSession(params.SessionID)
		if err != nil {
			return nil, true, err
		}
		return runtimeResult(sess), true, nil

	case methodSessionSetReasoning:
		var params ProtonmanSessionSetReasoningParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, true, fmt.Errorf("decode %s: %w", methodSessionSetReasoning, err)
		}
		sess, err := s.runtimeSession(params.SessionID)
		if err != nil {
			return nil, true, err
		}
		effort, err := domain.ParseReasoningEffort(params.Reasoning)
		if err != nil {
			return nil, true, err
		}
		if err := s.setSessionReasoning(ctx, sess, effort); err != nil {
			return nil, true, err
		}
		return runtimeResult(sess), true, nil

	case methodSessionSetLowConcurrency:
		var params ProtonmanSessionSetLowConcurrencyParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, true, fmt.Errorf("decode %s: %w", methodSessionSetLowConcurrency, err)
		}
		sess, err := s.runtimeSession(params.SessionID)
		if err != nil {
			return nil, true, err
		}
		setting, err := modelconfig.ParseLowConcurrencySetting(params.LowConcurrency)
		if err != nil {
			return nil, true, err
		}
		if err := s.updateSessionRuntime(ctx, sess, func(next *SessionRuntimeSettings) {
			next.LowConcurrency = setting.String()
		}); err != nil {
			return nil, true, err
		}
		return runtimeResult(sess), true, nil

	case methodSessionSetModel:
		var params ProtonmanSessionSetModelParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, true, fmt.Errorf("decode %s: %w", methodSessionSetModel, err)
		}
		sess, err := s.runtimeSession(params.SessionID)
		if err != nil {
			return nil, true, err
		}
		provider := strings.TrimSpace(params.Provider)
		modelID := strings.TrimSpace(params.Model)
		if provider == "" || modelID == "" {
			return nil, true, fmt.Errorf("provider and model are required")
		}
		if err := s.updateSessionRuntime(ctx, sess, func(next *SessionRuntimeSettings) {
			next.Provider = provider
			next.Model = modelID
		}); err != nil {
			return nil, true, err
		}
		return runtimeResult(sess), true, nil
	default:
		return nil, false, nil
	}
}

func (s *Server) runtimeSession(sessionID string) (*Session, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("sessionId is required")
	}
	sess, ok := s.lookupSession(sessionID)
	if !ok {
		return nil, fmt.Errorf("unknown session %q", sessionID)
	}
	return sess, nil
}

func (s *Server) setSessionReasoning(ctx context.Context, sess *Session, effort domain.ReasoningEffort) error {
	if !effort.Valid() {
		return fmt.Errorf("invalid reasoning effort %q", effort)
	}
	sess.mu.Lock()
	if sess.active {
		sess.mu.Unlock()
		return fmt.Errorf("session %q has an active prompt", sess.id)
	}
	clone, err := app.CloneConversationWithReasoning(sess.runner, effort, effort != domain.ReasoningDefault)
	if err == nil {
		sess.runner = clone
		sess.reasoningEffort = effort
		settings := sessionRuntimeForLocked(sess)
		settings.Reasoning = reasoningSetting(effort)
		storeSessionRuntime(sess, settings)
		sess.mu.Unlock()
		return sess.saveStateDetached(ctx)
	}
	sess.mu.Unlock()

	// If the runner cannot be cloned directly (e.g. mock or custom runner in tests),
	// fall back to rebuilding the conversation through session runtime controls.
	if control, ok := sessionRuntimeControlFor(s); ok && control.build != nil {
		return s.updateSessionRuntime(ctx, sess, func(next *SessionRuntimeSettings) {
			next.Reasoning = reasoningSetting(effort)
		})
	}
	return err
}

func (s *Server) updateSessionRuntime(ctx context.Context, sess *Session, mutate func(*SessionRuntimeSettings)) error {
	control, ok := sessionRuntimeControlFor(s)
	if !ok || control.build == nil {
		return fmt.Errorf("session runtime reconfiguration is unavailable")
	}

	sess.mu.Lock()
	if sess.active {
		sess.mu.Unlock()
		return fmt.Errorf("session %q has an active prompt", sess.id)
	}
	next := sessionRuntimeForLocked(sess)
	if mutate != nil {
		mutate(&next)
	}
	next = normalizeSessionRuntime(next)
	if _, err := domain.ParseReasoningEffort(next.Reasoning); err != nil {
		sess.mu.Unlock()
		return err
	}
	if _, err := modelconfig.ParseLowConcurrencySetting(next.LowConcurrency); err != nil {
		sess.mu.Unlock()
		return err
	}

	runner, err := control.build(ctx, sess.id, sess.cwd, sess.service, sess.agents, next)
	if err != nil {
		sess.mu.Unlock()
		return err
	}
	if runner == nil {
		sess.mu.Unlock()
		return fmt.Errorf("runtime builder returned no conversation")
	}

	effort, _ := domain.ParseReasoningEffort(next.Reasoning)
	sess.runner = runner
	sess.reasoningEffort = effort
	storeSessionRuntime(sess, next)
	sess.mu.Unlock()
	return sess.saveStateDetached(ctx)
}

func (s *Server) reconfigureSessionRuntime(ctx context.Context, sess *Session, next SessionRuntimeSettings) error {
	next = normalizeSessionRuntime(next)
	return s.updateSessionRuntime(ctx, sess, func(current *SessionRuntimeSettings) {
		*current = next
	})
}

func bindSessionRuntime(server *Server, sess *Session) {
	control, ok := sessionRuntimeControlFor(server)
	if !ok {
		return
	}
	key := weak.Make(sess)
	runtime.AddCleanup(sess, func(key weak.Pointer[Session]) {
		sessionRuntimeSelections.Delete(key)
	}, key)
	settings := control.defaults
	settings.Reasoning = reasoningSetting(sess.ReasoningEffort())
	storeSessionRuntime(sess, settings)
	runtime.KeepAlive(sess)
}

func restoreSessionRuntime(ctx context.Context, server *Server, sess *Session, persisted session.State) error {
	current := sessionRuntimeFor(sess)
	settings := current
	if strings.TrimSpace(persisted.ModelProvider) != "" {
		settings.Provider = persisted.ModelProvider
	}
	if strings.TrimSpace(persisted.ModelID) != "" {
		settings.Model = persisted.ModelID
	}
	if strings.TrimSpace(persisted.LowConcurrencyMode) != "" {
		setting, err := modelconfig.ParseLowConcurrencySetting(persisted.LowConcurrencyMode)
		if err != nil {
			return err
		}
		settings.LowConcurrency = setting.String()
	}
	settings.Reasoning = reasoningSetting(sess.ReasoningEffort())
	if settings.Provider == current.Provider && settings.Model == current.Model && settings.LowConcurrency == current.LowConcurrency {
		storeSessionRuntime(sess, settings)
		return nil
	}
	return server.reconfigureSessionRuntime(ctx, sess, settings)
}

func sessionRuntimeControlFor(server *Server) (sessionRuntimeControl, bool) {
	value, ok := sessionRuntimeControls.Load(server)
	if !ok {
		return sessionRuntimeControl{}, false
	}
	control, ok := value.(sessionRuntimeControl)
	return control, ok
}

func sessionRuntimeFor(sess *Session) SessionRuntimeSettings {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return sessionRuntimeForLocked(sess)
}

func sessionRuntimeForLocked(sess *Session) SessionRuntimeSettings {
	if value, ok := sessionRuntimeSelections.Load(weak.Make(sess)); ok {
		if settings, ok := value.(SessionRuntimeSettings); ok {
			settings.Reasoning = reasoningSetting(sess.reasoningEffort)
			return normalizeSessionRuntime(settings)
		}
	}
	return SessionRuntimeSettings{Reasoning: reasoningSetting(sess.reasoningEffort), LowConcurrency: "auto"}
}

func storeSessionRuntime(sess *Session, settings SessionRuntimeSettings) {
	sessionRuntimeSelections.Store(weak.Make(sess), normalizeSessionRuntime(settings))
}

func runtimeResult(sess *Session) ProtonmanSessionRuntimeResult {
	return ProtonmanSessionRuntimeResult{SessionID: sess.id, SessionRuntimeSettings: sessionRuntimeFor(sess)}
}

func normalizeSessionRuntime(settings SessionRuntimeSettings) SessionRuntimeSettings {
	settings.Provider = strings.TrimSpace(settings.Provider)
	settings.Model = strings.TrimSpace(settings.Model)
	if effort, err := domain.ParseReasoningEffort(settings.Reasoning); err == nil {
		settings.Reasoning = reasoningSetting(effort)
	} else {
		settings.Reasoning = strings.TrimSpace(settings.Reasoning)
	}
	if low, err := modelconfig.ParseLowConcurrencySetting(settings.LowConcurrency); err == nil {
		settings.LowConcurrency = low.String()
	} else {
		settings.LowConcurrency = strings.TrimSpace(settings.LowConcurrency)
	}
	return settings
}

func reasoningSetting(effort domain.ReasoningEffort) string {
	if effort == domain.ReasoningDefault {
		return "auto"
	}
	return string(effort)
}
