package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/in/acp"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

func acpSessionRuntimeOption(runtimeState *appRuntime) acp.Option {
	lowConcurrency := strings.TrimSpace(runtimeState.state.LowConcurrencyMode)
	if lowConcurrency == "" {
		lowConcurrency = "auto"
	}
	defaults := acp.SessionRuntimeSettings{
		Provider:       runtimeState.config.Model.Provider,
		Model:          runtimeState.config.Model.Default,
		Reasoning:      reasoningSetting(runtimeState.config.Agent.ReasoningEffort),
		LowConcurrency: lowConcurrency,
	}
	return acp.WithSessionRuntimeControls(defaults, func(
		ctx context.Context,
		sessionID string,
		cwd string,
		service *toolcall.Service,
		agents app.Agents,
		settings acp.SessionRuntimeSettings,
	) (app.Conversation, error) {
		providerName := strings.ToLower(strings.TrimSpace(settings.Provider))
		if providerName == "" {
			providerName = strings.ToLower(strings.TrimSpace(runtimeState.config.Model.Provider))
		}
		if providerName == "" {
			providerName = "protonman"
		}
		provider, ok := runtimeState.config.Providers[providerName]
		if !ok {
			defaultProvider := strings.ToLower(strings.TrimSpace(runtimeState.config.Model.Provider))
			if defaultProvider != "" && defaultProvider != providerName {
				if p, fallbackOK := runtimeState.config.Providers[defaultProvider]; fallbackOK {
					provider = p
					providerName = defaultProvider
					ok = true
				}
			}
			if !ok {
				return nil, fmt.Errorf("provider %q is not configured", settings.Provider)
			}
		}
		modelID := strings.TrimSpace(settings.Model)
		if modelID == "" {
			return nil, fmt.Errorf("model is required")
		}
		effort, err := domain.ParseReasoningEffort(settings.Reasoning)
		if err != nil {
			return nil, err
		}
		low, err := model.ParseLowConcurrencySetting(settings.LowConcurrency)
		if err != nil {
			return nil, err
		}

		goal := ""
		if runtimeState.stateStore != nil {
			state, found, loadErr := runtimeState.stateStore.Load(ctx, sessionID)
			if loadErr != nil {
				return nil, fmt.Errorf("load session runtime state: %w", loadErr)
			}
			if found {
				goal = state.ActiveGoal
			}
		}

		return app.BuildConversation(service, runtimeState.skills, agents, app.ConversationSpec{
			ProviderName:    providerName,
			ProviderType:    provider.Type,
			BaseURL:         provider.BaseURL,
			APIKey:          provider.APIKey,
			ModelID:         modelID,
			SessionID:       sessionID,
			Workspace:       cwd,
			ActiveGoal:      goal,
			AgentProfile:    runtimeState.config.Agent.Profile,
			ReasoningEffort: effort,
			MaxToolCalls:    runtimeState.config.Agent.MaxToolCalls,
			RequestTimeout:  runtimeState.config.Runtime.ModelRequestTimeout,
			TurnTimeout:     runtimeState.config.Runtime.TurnTimeout,
			RoundTimeout:    runtimeState.config.Runtime.RoundTimeout,
			ModelFactory:    runtimeState.application.ModelFactory,
			LowConcurrency:  low,
		})
	})
}

func acpSessionModelOptionsProvider(runtimeState *appRuntime) acp.SessionModelOptionsProvider {
	var mu sync.Mutex
	type cachedModels struct {
		options   []acp.SessionConfigSelectOption
		fetchedAt time.Time
	}
	cache := make(map[string]cachedModels)
	const cacheTTL = 5 * time.Minute

	return func(ctx context.Context, settings acp.SessionRuntimeSettings) ([]acp.SessionConfigSelectOption, error) {
		providerName := strings.ToLower(strings.TrimSpace(settings.Provider))
		if providerName == "" {
			providerName = strings.ToLower(strings.TrimSpace(runtimeState.config.Model.Provider))
		}
		if providerName == "" {
			providerName = "protonman"
		}

		mu.Lock()
		if entry, ok := cache[providerName]; ok && time.Since(entry.fetchedAt) < cacheTTL {
			cached := append([]acp.SessionConfigSelectOption(nil), entry.options...)
			mu.Unlock()
			return cached, nil
		}
		mu.Unlock()

		provider, hasProvider := runtimeState.config.Providers[providerName]
		var remoteModels []modelcatalog.RemoteModel
		var discoverErr error

		if hasProvider && runtimeState.application.Models != (app.Models{}) {
			discCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			remoteModels, discoverErr = runtimeState.application.Models.Discover(discCtx, app.ModelDiscoveryRequest{
				ProviderName: providerName,
				ProviderType: provider.Type,
				BaseURL:      provider.BaseURL,
				APIKey:       provider.APIKey,
				Timeout:      3 * time.Second,
			})
			cancel()
		}

		options := make([]acp.SessionConfigSelectOption, 0, len(remoteModels)+5)
		seen := make(map[string]bool)
		for _, m := range remoteModels {
			id := strings.TrimSpace(m.ID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			name := strings.TrimSpace(m.Name)
			if name == "" {
				name = id
			}
			options = append(options, acp.SessionConfigSelectOption{
				Value: id,
				Name:  name,
			})
		}

		if len(options) == 0 {
			fallback := providerFallbackModels(providerName)
			for _, item := range fallback {
				if !seen[item.Value] {
					seen[item.Value] = true
					options = append(options, item)
				}
			}
		}

		currentModel := strings.TrimSpace(settings.Model)
		if currentModel != "" && !seen[currentModel] {
			options = append([]acp.SessionConfigSelectOption{{Value: currentModel, Name: currentModel}}, options...)
		}

		mu.Lock()
		cache[providerName] = cachedModels{
			options:   append([]acp.SessionConfigSelectOption(nil), options...),
			fetchedAt: time.Now(),
		}
		mu.Unlock()
		if len(options) > 0 {
			return options, nil
		}
		return options, discoverErr
	}
}

func providerFallbackModels(provider string) []acp.SessionConfigSelectOption {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "protonman":
		return []acp.SessionConfigSelectOption{
			{Value: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet"},
			{Value: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet"},
			{Value: "gpt-4o", Name: "GPT-4o"},
			{Value: "o3-mini", Name: "o3-mini"},
			{Value: "deepseek-reasoner", Name: "DeepSeek-R1"},
		}
	case "openai":
		return []acp.SessionConfigSelectOption{
			{Value: "gpt-4o", Name: "GPT-4o"},
			{Value: "gpt-4o-mini", Name: "GPT-4o Mini"},
			{Value: "o3-mini", Name: "o3-mini"},
			{Value: "o1", Name: "o1"},
		}
	case "anthropic":
		return []acp.SessionConfigSelectOption{
			{Value: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet"},
			{Value: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet"},
			{Value: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku"},
		}
	case "opencode":
		return []acp.SessionConfigSelectOption{
			{Value: "nemotron-3-super-free", Name: "Nemotron 3 Super Free"},
			{Value: "big-pickle", Name: "Big Pickle"},
			{Value: "mimo-v2.5-free", Name: "MiMo V2.5 Free"},
		}
	default:
		return []acp.SessionConfigSelectOption{
			{Value: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet"},
			{Value: "gpt-4o", Name: "GPT-4o"},
		}
	}
}
