package modelsetup

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func ActiveProviderName(names []string, index int) string {
	if index < 0 || index >= len(names) {
		return model.DefaultOpenCodeName
	}
	return names[index]
}

func ReasoningIndex(choices []domain.ReasoningEffort, desired domain.ReasoningEffort) int {
	for i, effort := range choices {
		if effort == desired {
			return i
		}
	}
	return 0
}

func SelectedReasoning(choices []domain.ReasoningEffort, index int) domain.ReasoningEffort {
	if index < 0 || index >= len(choices) {
		return domain.ReasoningDefault
	}
	return choices[index]
}

func MoveReasoning(choices []domain.ReasoningEffort, index, delta int) (int, domain.ReasoningEffort) {
	if len(choices) == 0 {
		return 0, domain.ReasoningDefault
	}
	index = (index + delta + len(choices)) % len(choices)
	return index, choices[index]
}
