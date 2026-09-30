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
	"github.com/phongsathornpt/protonman/internal/core/modelconfig"
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
			if preset := model.LookupPreset(providerName); preset != nil {
				provider = modelconfig.Provider{
					Name:    preset.ID,
					Type:    string(preset.Protocol),
					BaseURL: preset.BaseURL,
				}
				ok = true
			}
		}
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
		_ = runtimeState.application.Providers.Activate(providerName, modelID)
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
	case "opencode-zen":
		return []acp.SessionConfigSelectOption{
			{Value: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet"},
			{Value: "gpt-4o", Name: "GPT-4o"},
			{Value: "o3-mini", Name: "o3-mini"},
		}
	case "opencode-go":
		return []acp.SessionConfigSelectOption{
			{Value: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet"},
			{Value: "gpt-4o", Name: "GPT-4o"},
		}
	case "ollama":
		return []acp.SessionConfigSelectOption{
			{Value: "llama3.3", Name: "Llama 3.3"},
			{Value: "qwen2.5-coder", Name: "Qwen 2.5 Coder"},
			{Value: "deepseek-r1", Name: "DeepSeek R1"},
		}
	default:
		return []acp.SessionConfigSelectOption{
			{Value: "claude-3-7-sonnet-20250219", Name: "Claude 3.7 Sonnet"},
			{Value: "gpt-4o", Name: "GPT-4o"},
		}
	}
}

func acpProvidersControl(runtimeState *appRuntime) acp.ProvidersControl {
	return acp.ProvidersControl{
		List: func(ctx context.Context) (acp.ProtonmanProvidersListResult, error) {
			activeProvider := strings.ToLower(strings.TrimSpace(runtimeState.config.Model.Provider))
			activeModel := strings.TrimSpace(runtimeState.config.Model.Default)

			configuredMap := make(map[string]bool)
			providers := make([]acp.ProviderInfo, 0, len(model.SupportedPresets)+len(runtimeState.config.Providers))

			// Add supported presets
			for _, preset := range model.SupportedPresets {
				idLower := strings.ToLower(preset.ID)
				configuredMap[idLower] = true
				cfg, isConfigured := runtimeState.config.Providers[idLower]
				baseURL := preset.BaseURL
				hasKey := false
				defaultModel := ""
				if isConfigured {
					if cfg.BaseURL != "" {
						baseURL = cfg.BaseURL
					}
					hasKey = strings.TrimSpace(cfg.APIKey) != ""
				}
				if idLower == activeProvider {
					defaultModel = activeModel
				}
				providers = append(providers, acp.ProviderInfo{
					ID:           preset.ID,
					Name:         preset.Name,
					Protocol:     string(preset.Protocol),
					BaseURL:      baseURL,
					RequiresKey:  preset.RequiresKey,
					IsConfigured: isConfigured,
					IsActive:     idLower == activeProvider,
					IsFree:       !preset.RequiresKey,
					HasKey:       hasKey,
					DefaultModel: defaultModel,
				})
			}

			// Add custom configured providers
			for name, cfg := range runtimeState.config.Providers {
				idLower := strings.ToLower(name)
				if configuredMap[idLower] {
					continue
				}
				configuredMap[idLower] = true
				proto := cfg.Type
				if proto == "" {
					proto = string(model.ProviderProtocolOpenAI)
				}
				defaultModel := ""
				if idLower == activeProvider {
					defaultModel = activeModel
				}
				providers = append(providers, acp.ProviderInfo{
					ID:           name,
					Name:         name,
					Protocol:     proto,
					BaseURL:      cfg.BaseURL,
					RequiresKey:  true,
					IsConfigured: true,
					IsActive:     idLower == activeProvider,
					IsFree:       false,
					HasKey:       strings.TrimSpace(cfg.APIKey) != "",
					DefaultModel: defaultModel,
				})
			}

			return acp.ProtonmanProvidersListResult{
				ActiveProvider: activeProvider,
				ActiveModel:    activeModel,
				Providers:      providers,
			}, nil
		},

		Save: func(ctx context.Context, params acp.ProtonmanProvidersSaveParams) (acp.ProtonmanProvidersSaveResult, error) {
			pName := strings.TrimSpace(params.ProviderName)
			pType := strings.TrimSpace(params.ProviderType)
			if pType == "" {
				if preset := model.LookupPreset(pName); preset != nil {
					pType = string(preset.Protocol)
				} else {
					pType = string(model.ProviderProtocolOpenAI)
				}
			}
			baseURL := model.ResolveProviderBaseURLForProtocol(pName, pType, params.BaseURL)
			saveReq := app.ProviderSaveRequest{
				Provider: modelconfig.Provider{
					Name:    pName,
					Type:    pType,
					BaseURL: baseURL,
					APIKey:  strings.TrimSpace(params.APIKey),
				},
				DefaultModel: strings.TrimSpace(params.DefaultModel),
				PreviousName: strings.TrimSpace(params.PreviousName),
				Activate:     params.Activate,
			}
			if err := runtimeState.application.Providers.Save(saveReq); err != nil {
				return acp.ProtonmanProvidersSaveResult{}, err
			}

			if runtimeState.config.Providers == nil {
				runtimeState.config.Providers = make(map[string]modelconfig.Provider)
			}
			runtimeState.config.Providers[strings.ToLower(pName)] = saveReq.Provider
			if saveReq.PreviousName != "" && !strings.EqualFold(saveReq.PreviousName, pName) {
				delete(runtimeState.config.Providers, strings.ToLower(saveReq.PreviousName))
			}
			if params.Activate {
				runtimeState.config.Model.Provider = pName
				if params.DefaultModel != "" {
					runtimeState.config.Model.Default = params.DefaultModel
				}
			}
			return acp.ProtonmanProvidersSaveResult{
				Success:        true,
				ActiveProvider: runtimeState.config.Model.Provider,
				ActiveModel:    runtimeState.config.Model.Default,
			}, nil
		},

		Delete: func(ctx context.Context, providerName string) error {
			cleanName := strings.TrimSpace(providerName)
			if err := runtimeState.application.Providers.Delete(cleanName); err != nil {
				return err
			}
			delete(runtimeState.config.Providers, strings.ToLower(cleanName))
			return nil
		},

		Models: func(ctx context.Context, params acp.ProtonmanProvidersModelsParams) ([]acp.SessionModelOption, error) {
			pName := strings.TrimSpace(params.ProviderName)
			pType := strings.TrimSpace(params.ProviderType)
			baseURL := strings.TrimSpace(params.BaseURL)
			apiKey := strings.TrimSpace(params.APIKey)

			if cfg, ok := runtimeState.config.Providers[strings.ToLower(pName)]; ok {
				if pType == "" {
					pType = cfg.Type
				}
				if baseURL == "" {
					baseURL = cfg.BaseURL
				}
				if apiKey == "" {
					apiKey = cfg.APIKey
				}
			}
			if preset := model.LookupPreset(pName); preset != nil {
				if pType == "" {
					pType = string(preset.Protocol)
				}
				if baseURL == "" {
					baseURL = preset.BaseURL
				}
			}
			baseURL = model.ResolveProviderBaseURLForProtocol(pName, pType, baseURL)

			var remoteModels []modelcatalog.RemoteModel
			var discoverErr error
			if runtimeState.application.Models != (app.Models{}) {
				discCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
				remoteModels, discoverErr = runtimeState.application.Models.Discover(discCtx, app.ModelDiscoveryRequest{
					ProviderName: pName,
					ProviderType: pType,
					BaseURL:      baseURL,
					APIKey:       apiKey,
					Timeout:      4 * time.Second,
				})
				cancel()
			}

			options := make([]acp.SessionModelOption, 0, len(remoteModels)+5)
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
				options = append(options, acp.SessionModelOption{
					ID:   id,
					Name: name,
				})
			}

			if len(options) == 0 {
				for _, fb := range providerFallbackModels(pName) {
					if !seen[fb.Value] {
						seen[fb.Value] = true
						options = append(options, acp.SessionModelOption{
							ID:   fb.Value,
							Name: fb.Name,
						})
					}
				}
			}
			if len(options) > 0 {
				return options, nil
			}
			return options, discoverErr
		},
	}
}
