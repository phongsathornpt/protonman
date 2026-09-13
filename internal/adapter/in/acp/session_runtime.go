package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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

var sessionRuntimeControls sync.Map // map[*Server]sessionRuntimeControl
var sessionRuntimeSelections sync.Map // map[*Session]SessionRuntimeSettings

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
		effort, err := sdk.ParseReasoningEffort(params.Reasoning)
		if err != nil {
			return nil, true, err
		}
		if err := sess.SetReasoningEffort(effort); err != nil {
			return nil, true, err
		}
		settings := sessionRuntimeFor(sess)
		settings.Reasoning = reasoningSetting(effort)
		storeSessionRuntime(sess, settings)
		if err := sess.saveStateDetached(ctx); err != nil {
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
		next := sessionRuntimeFor(sess)
		next.LowConcurrency = setting.String()
		if err := s.reconfigureSessionRuntime(ctx, sess, next); err != nil {
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
		next := sessionRuntimeFor(sess)
		next.Provider = provider
		next.Model = modelID
		if err := s.reconfigureSessionRuntime(ctx, sess, next); err != nil {
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

func (s *Server) reconfigureSessionRuntime(ctx context.Context, sess *Session, next SessionRuntimeSettings) error {
	control, ok := sessionRuntimeControlFor(s)
	if !ok || control.build == nil {
		return fmt.Errorf("session runtime reconfiguration is unavailable")
	}
	next = normalizeSessionRuntime(next)
	if _, err := sdk.ParseReasoningEffort(next.Reasoning); err != nil {
		return err
	}
	if _, err := modelconfig.ParseLowConcurrencySetting(next.LowConcurrency); err != nil {
		return err
	}

	sess.mu.Lock()
	if sess.active {
		sess.mu.Unlock()
		return fmt.Errorf("session %q has an active prompt", sess.id)
	}
	sess.mu.Unlock()

	runner, err := control.build(ctx, sess.id, sess.cwd, sess.service, sess.agents, next)
	if err != nil {
		return err
	}
	if runner == nil {
		return fmt.Errorf("runtime builder returned no conversation")
	}

	effort, _ := sdk.ParseReasoningEffort(next.Reasoning)
	sess.mu.Lock()
	if sess.active {
		sess.mu.Unlock()
		return fmt.Errorf("session %q became active while reconfiguring", sess.id)
	}
	sess.runner = runner
	sess.reasoningEffort = effort
	sess.mu.Unlock()
	storeSessionRuntime(sess, next)
	return sess.saveStateDetached(ctx)
}

func bindSessionRuntime(server *Server, sess *Session) {
	control, ok := sessionRuntimeControlFor(server)
	if !ok {
		return
	}
	settings := control.defaults
	settings.Reasoning = reasoningSetting(sess.ReasoningEffort())
	storeSessionRuntime(sess, settings)
}

func restoreSessionRuntime(ctx context.Context, server *Server, sess *Session, persisted session.State) error {
	settings := sessionRuntimeFor(sess)
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
	current := sessionRuntimeFor(sess)
	storeSessionRuntime(sess, settings)
	if settings.Provider == current.Provider && settings.Model == current.Model && settings.LowConcurrency == current.LowConcurrency {
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
	if value, ok := sessionRuntimeSelections.Load(sess); ok {
		if settings, ok := value.(SessionRuntimeSettings); ok {
			settings.Reasoning = reasoningSetting(sess.ReasoningEffort())
			return normalizeSessionRuntime(settings)
		}
	}
	return SessionRuntimeSettings{Reasoning: reasoningSetting(sess.ReasoningEffort()), LowConcurrency: "auto"}
}

func storeSessionRuntime(sess *Session, settings SessionRuntimeSettings) {
	sessionRuntimeSelections.Store(sess, normalizeSessionRuntime(settings))
}

func runtimeResult(sess *Session) ProtonmanSessionRuntimeResult {
	return ProtonmanSessionRuntimeResult{SessionID: sess.id, SessionRuntimeSettings: sessionRuntimeFor(sess)}
}

func normalizeSessionRuntime(settings SessionRuntimeSettings) SessionRuntimeSettings {
	settings.Provider = strings.TrimSpace(settings.Provider)
	settings.Model = strings.TrimSpace(settings.Model)
	if effort, err := sdk.ParseReasoningEffort(settings.Reasoning); err == nil {
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

func reasoningSetting(effort sdk.ReasoningEffort) string {
	if effort == sdk.ReasoningDefault {
		return "auto"
	}
	return string(effort)
}
