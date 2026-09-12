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
