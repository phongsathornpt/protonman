package modelsetup

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func ReasoningChoices(providerName string, md model.RemoteModel) []domain.ReasoningEffort {
	profile := model.ResolveModelProfile(providerName, md.ID, &md)
	choices := reasoningpolicy.Choices(profile)
	if len(choices) == 0 {
		return []domain.ReasoningEffort{domain.ReasoningDefault}
	}
	return choices
}

func CompatibleReasoning(providerName string, md model.RemoteModel, desired domain.ReasoningEffort) domain.ReasoningEffort {
	for _, effort := range ReasoningChoices(providerName, md) {
		if effort == desired {
			return desired
		}
	}
	return domain.ReasoningDefault
}
