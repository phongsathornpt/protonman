package modelsetup

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func ActiveProviderName(names []string, index int) string {
	if index < 0 || index >= len(names) {
		return model.DefaultOpenCodeName
	}
	return names[index]
}

func ReasoningIndex(choices []sdk.ReasoningEffort, desired sdk.ReasoningEffort) int {
	for i, effort := range choices {
		if effort == desired {
			return i
		}
	}
	return 0
}

func SelectedReasoning(choices []sdk.ReasoningEffort, index int) sdk.ReasoningEffort {
	if index < 0 || index >= len(choices) {
		return sdk.ReasoningDefault
	}
	return choices[index]
}

func MoveReasoning(choices []sdk.ReasoningEffort, index, delta int) (int, sdk.ReasoningEffort) {
	if len(choices) == 0 {
		return 0, sdk.ReasoningDefault
	}
	index = (index + delta + len(choices)) % len(choices)
	return index, choices[index]
}
