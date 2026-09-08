package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/adapter/out/config"
	"github.com/projectTHORN/proton/internal/adapter/out/model"
	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
	"github.com/projectTHORN/proton/internal/feature/agent"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// Agents owns inbound lifecycle/control access to the subagent coordinator.
type Agents struct{ coordinator *agent.Coordinator }

// SubagentModelResolverSpec contains immutable runtime inputs used to build
// configured per-profile subagent language models.
type SubagentModelResolverSpec struct {
	Providers      map[string]config.ProviderConfig
	Overrides      map[string]config.SubagentModelConfig
	SessionID      string
	RequestTimeout time.Duration
}

// BuildSubagentModelResolver validates configured provider/model pairs and
// prebuilds immutable language-model overrides. Missing profiles dynamically
// inherit the current Universal model inside the coordinator.
func BuildSubagentModelResolver(spec SubagentModelResolverSpec) (*agent.ModelResolver, error) {
	if len(spec.Overrides) == 0 {
		return nil, nil
	}
	overrides := make(map[agent.Profile]sdk.LanguageModel, len(spec.Overrides))
	for rawProfile, configured := range spec.Overrides {
		profile, err := agent.ParseSubagentProfile(rawProfile)
		if err != nil {
			return nil, fmt.Errorf("subagent model %q: %w", rawProfile, err)
		}
		if strings.TrimSpace(configured.Provider) == "" && strings.TrimSpace(configured.Model) == "" {
			continue
		}
		providerKey, provider, ok := lookupProvider(spec.Providers, configured.Provider)
		if !ok {
			return nil, fmt.Errorf("agent.subagents.%s: provider %q is not configured", profile, configured.Provider)
		}
		if !model.ProviderHasUsableAuth(providerKey, provider.BaseURL, provider.APIKey) {
			return nil, fmt.Errorf("agent.subagents.%s: provider %q requires credentials", profile, providerKey)
		}
		opts := []model.ClientOption{model.WithRequestTimeout(spec.RequestTimeout)}
		if strings.TrimSpace(spec.SessionID) != "" {
			opts = append(opts, model.WithSessionID(spec.SessionID))
		}
		overrides[profile] = model.NewProviderLanguageModel(
			providerKey, provider.Type, provider.BaseURL, provider.APIKey, configured.Model, opts...,
		)
	}
	if len(overrides) == 0 {
		return nil, nil
	}
	return agent.NewModelResolver(overrides)
}

// BuildSubagentReasoningResolver validates and snapshots per-profile reasoning overrides.
// Profiles configured as auto/default inherit the current global/profile policy.
func BuildSubagentReasoningResolver(configured map[string]config.SubagentModelConfig) (*agent.ReasoningResolver, error) {
	if len(configured) == 0 {
		return nil, nil
	}
	overrides := make(map[agent.Profile]sdk.ReasoningEffort, len(configured))
	for rawProfile, subagentConfig := range configured {
		profile, err := agent.ParseSubagentProfile(rawProfile)
		if err != nil {
			return nil, fmt.Errorf("subagent reasoning %q: %w", rawProfile, err)
		}
		if subagentConfig.ReasoningEffort != sdk.ReasoningDefault {
			overrides[profile] = subagentConfig.ReasoningEffort
		}
	}
	if len(overrides) == 0 {
		return nil, nil
	}
	return agent.NewReasoningResolver(overrides)
}

func lookupProvider(providers map[string]config.ProviderConfig, requested string) (string, config.ProviderConfig, bool) {
	requested = strings.TrimSpace(requested)
	if provider, ok := providers[requested]; ok {
		return requested, provider, true
	}
	for key, provider := range providers {
		if strings.EqualFold(key, requested) {
			return key, provider, true
		}
	}
	return "", config.ProviderConfig{}, false
}

func NewAgents(coordinator *agent.Coordinator) Agents { return Agents{coordinator: coordinator} }
func (a Agents) Available() bool                      { return a.coordinator != nil }
func (a Agents) Subscribe(buffer int) (<-chan agent.Event, func()) {
	if a.coordinator == nil {
		ch := make(chan agent.Event)
		close(ch)
		return ch, func() {}
	}
	return a.coordinator.Subscribe(buffer)
}
func (a Agents) Enabled() bool { return a.coordinator != nil && a.coordinator.Enabled() }
func (a Agents) SetEnabled(enabled bool) {
	if a.coordinator != nil {
		a.coordinator.SetEnabled(enabled)
	}
}
func (a Agents) List() []agent.AgentStatus {
	if a.coordinator == nil {
		return nil
	}
	return a.coordinator.List()
}
func (a Agents) CancelByParent(parentID string) int {
	if a.coordinator == nil {
		return 0
	}
	return a.coordinator.CancelByParent(parentID)
}
func (a Agents) SetLanguageModel(languageModel sdk.LanguageModel) {
	if a.coordinator != nil {
		a.coordinator.SetLanguageModel(languageModel)
	}
}

func (a Agents) SetPermissionMode(mode permission.Mode) {
	if a.coordinator != nil {
		a.coordinator.SetPermissionMode(mode)
	}
}
func (a Agents) SetReasoningEffort(effort sdk.ReasoningEffort) {
	if a.coordinator != nil {
		a.coordinator.SetReasoningEffort(effort)
	}
}
func (a Agents) SetPrompt(prompt toolcall.PermissionPrompt) {
	if a.coordinator != nil {
		a.coordinator.SetPrompt(prompt)
	}
}
func (a Agents) SetCallGuard(guard toolcall.CallGuard) {
	if a.coordinator != nil {
		a.coordinator.SetCallGuard(guard)
	}
}
