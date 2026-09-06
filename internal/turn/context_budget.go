package turn

import (
	"encoding/json"
	"fmt"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

const estimatedBytesPerToken = 3

func validateContextBudget(languageModel sdk.LanguageModel, request sdk.Request) error {
	window := sdk.ModelContextWindow(languageModel)
	if window <= 0 {
		return nil
	}
	estimated, err := estimateRequestTokens(request)
	if err != nil {
		return fmt.Errorf("estimate model context: %w", err)
	}
	reserve := contextOutputReserve(window, request.Options.MaxOutputTokens)
	if estimated+reserve <= window {
		return nil
	}
	return fmt.Errorf("%w: model %q estimated input %d tokens plus %d reserved output exceeds %d-token context window", ErrContextBudgetExceeded, languageModel.ModelID(), estimated, reserve, window)
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
