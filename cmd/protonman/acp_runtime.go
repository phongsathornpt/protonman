package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/in/acp"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
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
		provider, ok := runtimeState.config.Providers[providerName]
		if !ok {
			return nil, fmt.Errorf("provider %q is not configured", settings.Provider)
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
