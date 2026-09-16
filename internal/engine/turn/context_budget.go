package turn

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/conversation"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
	_ "golang.org/x/image/webp"
)

const estimatedBytesPerToken = 3

func validateContextBudget(languageModel port.LanguageModel, request domain.Request) error {
	return validateContextBudgetWithVisionPolicy(languageModel, request, modelprofile.DefaultVisionPolicy())
}

func validateContextBudgetWithVisionPolicy(languageModel port.LanguageModel, request domain.Request, visionPolicy modelprofile.VisionPolicy) error {
	limits := usecase.ModelTokenLimits(languageModel)
	if limits.ContextWindow <= 0 && limits.MaxInputTokens <= 0 && limits.MaxOutputTokens <= 0 {
		return nil
	}
	estimated, err := estimateRequestTokensWithVisionPolicy(request, visionPolicy)
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

func estimateRequestTokens(request domain.Request) (int, error) {
	return estimateRequestTokensWithVisionPolicy(request, modelprofile.DefaultVisionPolicy())
}

func estimateRequestTokensWithVisionPolicy(request domain.Request, visionPolicy modelprofile.VisionPolicy) (int, error) {
	visionPolicy = modelprofile.EffectiveVisionPolicy(modelprofile.Resolved{VisionPolicy: visionPolicy})
	transportNeutral := request
	transportNeutral.Messages = domain.CloneMessages(request.Messages)
	imageTokens := 0
	for messageIndex := range transportNeutral.Messages {
		for partIndex := range transportNeutral.Messages[messageIndex].Parts {
			part := &transportNeutral.Messages[messageIndex].Parts[partIndex]
			if part.Type != domain.ContentPartImage {
				continue
			}
			imageTokens += estimateImageTokensWithPolicy(part.Data, visionPolicy)
			// Base64 is a transport representation, not text consumed by the model.
			// Keep MIME/type framing in the serialized estimate but remove payload bytes.
			part.Data = ""
		}
	}

	payload, err := json.Marshal(transportNeutral)
	if err != nil {
		return 0, err
	}
	textAndFramingTokens := 0
	if len(payload) > 0 {
		textAndFramingTokens = (len(payload) + estimatedBytesPerToken - 1) / estimatedBytesPerToken
	}
	return textAndFramingTokens + imageTokens, nil
}

func estimateImageTokens(data string) int {
	return estimateImageTokensWithPolicy(data, modelprofile.DefaultVisionPolicy())
}

func estimateImageTokensWithPolicy(data string, policy modelprofile.VisionPolicy) int {
	policy = modelprofile.EffectiveVisionPolicy(modelprofile.Resolved{VisionPolicy: policy})
	data = strings.TrimSpace(data)
	if data == "" {
		return policy.FallbackTokens
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(data))
	config, _, err := image.DecodeConfig(decoder)
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return policy.FallbackTokens
	}

	switch policy.TokenScheme {
	case modelprofile.VisionTokenAnthropicPixels:
		// Anthropic documents an approximate width*height/750 image-token rule.
		pixels := int64(config.Width) * int64(config.Height)
		tokens := int((pixels + 749) / 750)
		return max(1, tokens)
	case modelprofile.VisionTokenGeminiTiles:
		// Gemini images up to 384px in both dimensions cost 258 tokens. Larger
		// images are tiled; Google's published rough crop unit is floor(min/1.5).
		if config.Width <= 384 && config.Height <= 384 {
			return 258
		}
		minSide := min(config.Width, config.Height)
		crop := max(1, (2*minSide)/3)
		tilesWide := ceilDiv(config.Width, crop)
		tilesHigh := ceilDiv(config.Height, crop)
		return max(258, tilesWide*tilesHigh*258)
	default:
		patch := max(1, policy.PatchSize)
		patchesWide := ceilDiv(config.Width, patch)
		patchesHigh := ceilDiv(config.Height, patch)
		patches := patchesWide * patchesHigh
		if patches < 1 {
			return 1
		}
		if policy.MaxPatches > 0 && patches > policy.MaxPatches {
			return policy.MaxPatches
		}
		return patches
	}
}

func ceilDiv(value, divisor int) int {
	if divisor <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
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

func effectiveInputBudget(limits domain.TokenLimits, requestedOutput int) int {
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

func compactRequestToModelBudget(request domain.Request, limits domain.TokenLimits, policy modelprofile.CompactionPolicy) (domain.Request, conversation.CompactionDecision, error) {
	return compactRequestToModelBudgetWithVisionPolicy(request, limits, policy, modelprofile.DefaultVisionPolicy())
}

func compactRequestToModelBudgetWithVisionPolicy(request domain.Request, limits domain.TokenLimits, policy modelprofile.CompactionPolicy, visionPolicy modelprofile.VisionPolicy) (domain.Request, conversation.CompactionDecision, error) {
	estimated, err := estimateRequestTokensWithVisionPolicy(request, visionPolicy)
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
	fixedTokens, err := estimateRequestTokensWithVisionPolicy(fixed, visionPolicy)
	if err != nil {
		return request, decision, err
	}
	messageTarget := decision.TargetTokens - fixedTokens
	if messageTarget < 1 {
		messageTarget = 1
	}

	retained, err := conversation.RetainByCost(
		request.Messages,
		policy.MinRecentMessages,
		2048,
		messageTarget,
		func(messages []domain.Message) (int, error) {
			candidate := request
			candidate.Messages = messages
			tokens, err := estimateRequestTokensWithVisionPolicy(candidate, visionPolicy)
			if err != nil {
				return 0, err
			}
			if tokens <= fixedTokens {
				return 0, nil
			}
			return tokens - fixedTokens, nil
		},
	)
	if err != nil {
		return request, decision, err
	}
	request.Messages = retained
	return request, decision, nil
}
