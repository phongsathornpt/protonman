package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Agents owns inbound lifecycle/control access to the subagent coordinator.
type Agents struct {
	coordinator *agent.Coordinator
	sessionID   string
}

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

func NewAgentsForSession(coordinator *agent.Coordinator, sessionID string) Agents {
	return Agents{coordinator: coordinator, sessionID: strings.TrimSpace(sessionID)}
}
func (a Agents) ForSession(sessionID string) Agents {
	a.sessionID = strings.TrimSpace(sessionID)
	return a
}
func (a Agents) Available() bool { return a.coordinator != nil }
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
	if a.sessionID != "" {
		return a.coordinator.ListSession(a.sessionID)
	}
	return a.coordinator.List()
}
func (a Agents) CancelSessionAndWait(ctx context.Context) (int, error) {
	if a.coordinator == nil {
		return 0, nil
	}
	return a.coordinator.CancelSessionAndWait(ctx, a.sessionID)
}
func (a Agents) CancelTurn(parentID string, policy agent.CancelPolicy) int {
	if a.coordinator == nil {
		return 0
	}
	return a.coordinator.CancelTurn(agent.TurnRef{SessionID: a.sessionID, TurnID: parentID}, policy)
}
func (a Agents) CancelByParent(parentID string) int {
	if a.coordinator == nil {
		return 0
	}
	if a.sessionID != "" {
		return a.coordinator.CancelByTurn(agent.TurnRef{SessionID: a.sessionID, TurnID: parentID})
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
