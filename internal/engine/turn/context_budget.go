package turn

import (
	"encoding/json"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/core/conversation"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const estimatedBytesPerToken = 3

func validateContextBudget(languageModel sdk.LanguageModel, request sdk.Request) error {
	limits := sdk.ModelTokenLimits(languageModel)
	if limits.ContextWindow <= 0 && limits.MaxInputTokens <= 0 && limits.MaxOutputTokens <= 0 {
		return nil
	}
	estimated, err := estimateRequestTokens(request)
	if err != nil {
		return fmt.Errorf("estimate model context: %w", err)
	}
	if limits.MaxInputTokens > 0 && estimated > limits.MaxInputTokens {
		return fmt.Errorf("%w: model %q estimated input %d tokens exceeds %d-token input limit", ErrContextBudgetExceeded, languageModel.ModelID(), estimated, limits.MaxInputTokens)
	}
	requestedOutput := request.Options.MaxOutputTokens
	if limits.MaxOutputTokens > 0 && requestedOutput > limits.MaxOutputTokens {
		return fmt.Errorf("%w: model %q requested output %d tokens exceeds %d-token output limit", ErrContextBudgetExceeded, languageModel.ModelID(), requestedOutput, limits.MaxOutputTokens)
	}
	if limits.ContextWindow <= 0 {
		return nil
	}
	reserve := contextOutputReserve(limits.ContextWindow, requestedOutput)
	if requestedOutput == 0 && limits.MaxOutputTokens > 0 && reserve > limits.MaxOutputTokens {
		reserve = limits.MaxOutputTokens
	}
	if estimated+reserve <= limits.ContextWindow {
		return nil
	}
	return fmt.Errorf("%w: model %q estimated input %d tokens plus %d reserved output exceeds %d-token context window", ErrContextBudgetExceeded, languageModel.ModelID(), estimated, reserve, limits.ContextWindow)
}

func estimateRequestTokens(request sdk.Request) (int, error) {
	payload, err := json.Marshal(request)
	if err != nil {
		return 0, err
	}
	if len(payload) == 0 {
		return 0, nil
	}
	return (len(payload) + estimatedBytesPerToken - 1) / estimatedBytesPerToken, nil
}

func contextOutputReserve(window, requested int) int {
	if requested > 0 {
		if requested >= window {
			return window
		}
		return requested
	}
	reserve := window / 8
	if reserve < 256 {
		reserve = 256
	}
	if reserve > 8192 {
		reserve = 8192
	}
	if reserve >= window {
		reserve = window / 4
	}
	return reserve
}

func effectiveInputBudget(limits sdk.TokenLimits, requestedOutput int) int {
	budget := limits.MaxInputTokens
	if limits.ContextWindow > 0 {
		reserve := contextOutputReserve(limits.ContextWindow, requestedOutput)
		if requestedOutput == 0 && limits.MaxOutputTokens > 0 && reserve > limits.MaxOutputTokens {
			reserve = limits.MaxOutputTokens
		}
		contextBudget := limits.ContextWindow - reserve
		if contextBudget < 0 {
			contextBudget = 0
		}
		if budget <= 0 || contextBudget < budget {
			budget = contextBudget
		}
	}
	return budget
}

func compactRequestToModelBudget(request sdk.Request, limits sdk.TokenLimits, policy modelprofile.CompactionPolicy) (sdk.Request, conversation.CompactionDecision, error) {
	estimated, err := estimateRequestTokens(request)
	if err != nil {
		return request, conversation.CompactionDecision{}, err
	}
	budget := effectiveInputBudget(limits, request.Options.MaxOutputTokens)
	decision := conversation.PlanCompaction(estimated, budget, policy)
	if !decision.Required() || len(request.Messages) == 0 {
		return request, decision, nil
	}
	fixed := request
	fixed.Messages = nil
	fixedTokens, err := estimateRequestTokens(fixed)
	if err != nil {
		return request, decision, err
	}
	messageTarget := decision.TargetTokens - fixedTokens
	if messageTarget < 1 {
		messageTarget = 1
	}
	request.Messages = conversation.Retain(request.Messages, conversation.RetentionPolicy{
		MaxBytes:                     messageTarget * estimatedBytesPerToken,
		RecentMessages:               policy.MinRecentMessages,
		MaxHistoricalToolResultBytes: 2048,
	})
	return request, decision, nil
}
