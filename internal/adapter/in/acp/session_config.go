package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

const (
	methodSessionSetConfigOption = "session/set_config_option"

	configIDModel          = "model"
	configIDProvider       = "provider"
	configIDReasoning      = "reasoning"
	configIDLowConcurrency = "lowConcurrency"
)

// SessionConfigSelectOption is one selectable value in an ACP session config option.
type SessionConfigSelectOption struct {
	MetaCarrier
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SessionConfigOption is Protonman's v1 select-style ACP configuration option.
// All currently exposed options are selects, avoiding boolean capability gating
// while preserving the standard wire shape.
type SessionConfigOption struct {
	MetaCarrier
	ID           string                      `json:"id"`
	Name         string                      `json:"name"`
	Description  string                      `json:"description,omitempty"`
	Error        string                      `json:"error,omitempty"`
	Category     string                      `json:"category,omitempty"`
	Type         string                      `json:"type"`
	CurrentValue string                      `json:"currentValue"`
	Options      []SessionConfigSelectOption `json:"options"`
}

// SetSessionConfigOptionParams matches ACP v1 session/set_config_option for
// select/id values. Type is optional on v1 and defaults to a value-id payload.
type SetSessionConfigOptionParams struct {
	MetaCarrier
	SessionID string          `json:"sessionId"`
	ConfigID  string          `json:"configId"`
	Type      string          `json:"type,omitempty"`
	Value     json.RawMessage `json:"value"`
}

type SetSessionConfigOptionResult struct {
	MetaCarrier
	ConfigOptions []SessionConfigOption `json:"configOptions"`
}

// SessionModelOptionsProvider supplies models for the active provider. It is
// deliberately a callback so the ACP adapter stays independent of provider HTTP
// discovery and caching policy.
type SessionModelOptionsProvider func(context.Context, SessionRuntimeSettings) ([]SessionConfigSelectOption, error)

type SessionProviderOptionsProvider func(context.Context, SessionRuntimeSettings) ([]SessionConfigSelectOption, error)

var sessionModelOptionsProviders sync.Map    // map[*Server]SessionModelOptionsProvider
var sessionProviderOptionsProviders sync.Map // map[*Server]SessionProviderOptionsProvider

func WithSessionConfigModelOptions(provider SessionModelOptionsProvider) Option {
	return func(server *Server) {
		if provider != nil {
			sessionModelOptionsProviders.Store(server, provider)
		}
	}
}

func sessionModelOptionsProviderFor(server *Server) (SessionModelOptionsProvider, bool) {
	value, ok := sessionModelOptionsProviders.Load(server)
	if !ok {
		return nil, false
	}
	provider, ok := value.(SessionModelOptionsProvider)
	return provider, ok && provider != nil
}

func WithSessionConfigProviderOptions(provider SessionProviderOptionsProvider) Option {
	return func(server *Server) {
		if provider != nil {
			sessionProviderOptionsProviders.Store(server, provider)
		}
	}
}

func sessionProviderOptionsProviderFor(server *Server) (SessionProviderOptionsProvider, bool) {
	value, ok := sessionProviderOptionsProviders.Load(server)
	if !ok {
		return nil, false
	}
	provider, ok := value.(SessionProviderOptionsProvider)
	return provider, ok && provider != nil
}

func (s *Server) dispatchSessionConfig(ctx context.Context, request RPCRequest) (any, bool, error) {
	if request.Method != methodSessionSetConfigOption {
		return nil, false, nil
	}
	var params SetSessionConfigOptionParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		return nil, true, fmt.Errorf("decode %s: %w", methodSessionSetConfigOption, err)
	}
	sess, err := s.runtimeSession(params.SessionID)
	if err != nil {
		return nil, true, err
	}
	value, err := decodeSessionConfigStringValue(params)
	if err != nil {
		return nil, true, err
	}

	switch strings.TrimSpace(params.ConfigID) {
	case configIDProvider:
		options := s.sessionConfigOptions(ctx, sess)
		providerOption, ok := findSessionConfigOption(options, configIDProvider)
		if !ok || !selectOptionContains(providerOption, value) {
			return nil, true, fmt.Errorf("provider %q is not an advertised session config value", value)
		}
		modelID := sessionRuntimeFor(sess).Model
		if modelProvider, ok := sessionModelOptionsProviderFor(s); ok {
			next := sessionRuntimeFor(sess)
			next.Provider = value
			if discovered, err := modelProvider(ctx, next); err == nil && len(discovered) > 0 {
				modelID = strings.TrimSpace(discovered[0].Value)
			}
		}
		if err := s.updateSessionRuntime(ctx, sess, func(next *SessionRuntimeSettings) {
			next.Provider = value
			next.Model = modelID
		}); err != nil {
			return nil, true, err
		}

	case configIDModel:
		options := s.sessionConfigOptions(ctx, sess)
		modelOption, ok := findSessionConfigOption(options, configIDModel)
		if ok && modelOption.Error == "" && len(modelOption.Options) > 0 && !selectOptionContains(modelOption, value) {
			return nil, true, fmt.Errorf("model %q is not an advertised session config value", value)
		}
		if err := s.updateSessionRuntime(ctx, sess, func(next *SessionRuntimeSettings) {
			next.Model = value
		}); err != nil {
			return nil, true, err
		}

	case configIDReasoning, "thought_level", "thoughtLevel":
		effort, parseErr := domain.ParseReasoningEffort(value)
		if parseErr != nil {
			return nil, true, parseErr
		}
		if err := s.setSessionReasoning(ctx, sess, effort); err != nil {
			return nil, true, err
		}

	case configIDLowConcurrency, "low_concurrency":
		setting, parseErr := modelconfig.ParseLowConcurrencySetting(value)
		if parseErr != nil {
			return nil, true, parseErr
		}
		if err := s.updateSessionRuntime(ctx, sess, func(next *SessionRuntimeSettings) {
			next.LowConcurrency = setting.String()
		}); err != nil {
			return nil, true, err
		}

	default:
		return nil, true, fmt.Errorf("unknown session config option %q", params.ConfigID)
	}

	return SetSessionConfigOptionResult{ConfigOptions: s.sessionConfigOptions(ctx, sess)}, true, nil
}

func decodeSessionConfigStringValue(params SetSessionConfigOptionParams) (string, error) {
	kind := strings.TrimSpace(params.Type)
	if kind != "" && kind != "select" && kind != "value_id" {
		return "", fmt.Errorf("session config option %q requires a select value, got type %q", params.ConfigID, kind)
	}
	var value string
	if err := json.Unmarshal(params.Value, &value); err != nil {
		var objectValue struct {
			ID    string `json:"id"`
			Value string `json:"value"`
		}
		if objErr := json.Unmarshal(params.Value, &objectValue); objErr == nil {
			value = strings.TrimSpace(objectValue.Value)
			if value == "" {
				value = strings.TrimSpace(objectValue.ID)
			}
		}
		if value == "" {
			return "", fmt.Errorf("session config option %q requires a string value: %w", params.ConfigID, err)
		}
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("session config option %q value is required", params.ConfigID)
	}
	return value, nil
}

func (s *Server) sessionConfigOptions(ctx context.Context, sess *Session) []SessionConfigOption {
	if sess == nil {
		return nil
	}
	if _, ok := sessionRuntimeControlFor(s); !ok {
		return nil
	}
	settings := sessionRuntimeFor(sess)
	options := make([]SessionConfigOption, 0, 4)

	providerValues := []SessionConfigSelectOption{}
	if provider, ok := sessionProviderOptionsProviderFor(s); ok {
		if discovered, err := provider(ctx, settings); err == nil && len(discovered) > 0 {
			providerValues = ensureCurrentConfigValue(discovered, settings.Provider)
		}
	}
	if len(providerValues) > 0 {
		options = append(options, SessionConfigOption{
			ID:           configIDProvider,
			Name:         "Provider",
			Description:  "Provider used for future turns",
			Category:     "model",
			Type:         "select",
			CurrentValue: settings.Provider,
			Options:      providerValues,
		})
	}

	modelValues := []SessionConfigSelectOption{{Value: settings.Model, Name: settings.Model}}
	var modelError string
	if provider, ok := sessionModelOptionsProviderFor(s); ok {
		if discovered, err := provider(ctx, settings); err == nil && len(discovered) > 0 {
			modelValues = ensureCurrentConfigValue(discovered, settings.Model)
		} else if err != nil {
			modelError = err.Error()
		}
	}
	if strings.TrimSpace(settings.Model) != "" {
		options = append(options, SessionConfigOption{
			ID:           configIDModel,
			Name:         "Model",
			Description:  "Model used for future turns in the active provider",
			Error:        modelError,
			Category:     "model",
			Type:         "select",
			CurrentValue: settings.Model,
			Options:      modelValues,
		})
	}

	reasoningValues := []SessionConfigSelectOption{
		{Value: "auto", Name: "Auto"},
		{Value: "none", Name: "None"},
		{Value: "minimal", Name: "Minimal"},
		{Value: "low", Name: "Low"},
		{Value: "medium", Name: "Medium"},
		{Value: "high", Name: "High"},
		{Value: "xhigh", Name: "Extra High"},
		{Value: "max", Name: "Max"},
	}
	options = append(options, SessionConfigOption{
		ID:           configIDReasoning,
		Name:         "Reasoning",
		Description:  "Reasoning effort for future turns",
		Category:     "thought_level",
		Type:         "select",
		CurrentValue: settings.Reasoning,
		Options:      reasoningValues,
	})

	lowValues := []SessionConfigSelectOption{
		{Value: "auto", Name: "Auto"},
		{Value: "on", Name: "On"},
		{Value: "off", Name: "Off"},
	}
	options = append(options, SessionConfigOption{
		ID:           configIDLowConcurrency,
		Name:         "Low concurrency",
		Description:  "Conservative provider concurrency policy for future turns",
		Category:     "model_config",
		Type:         "select",
		CurrentValue: settings.LowConcurrency,
		Options:      lowValues,
	})
	return options
}

func ensureCurrentConfigValue(options []SessionConfigSelectOption, current string) []SessionConfigSelectOption {
	current = strings.TrimSpace(current)
	out := make([]SessionConfigSelectOption, 0, len(options)+1)
	seen := make(map[string]struct{}, len(options)+1)
	for _, option := range options {
		option.Value = strings.TrimSpace(option.Value)
		if option.Value == "" {
			continue
		}
		if option.Name == "" {
			option.Name = option.Value
		}
		if _, exists := seen[option.Value]; exists {
			continue
		}
		seen[option.Value] = struct{}{}
		out = append(out, option)
	}
	if current != "" {
		if _, exists := seen[current]; !exists {
			out = append([]SessionConfigSelectOption{{Value: current, Name: current}}, out...)
		}
	}
	return out
}

func findSessionConfigOption(options []SessionConfigOption, id string) (SessionConfigOption, bool) {
	for _, option := range options {
		if option.ID == id {
			return option, true
		}
	}
	return SessionConfigOption{}, false
}

func selectOptionContains(option SessionConfigOption, value string) bool {
	for _, candidate := range option.Options {
		if candidate.Value == value {
			return true
		}
	}
	return false
}
