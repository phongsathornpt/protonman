package modelsetup

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func ReasoningChoices(providerName string, md model.RemoteModel) []sdk.ReasoningEffort {
	profile := model.ResolveModelProfile(providerName, md.ID, &md)
	choices := reasoningpolicy.Choices(profile)
	if len(choices) == 0 {
		return []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	return choices
}

func CompatibleReasoning(providerName string, md model.RemoteModel, desired sdk.ReasoningEffort) sdk.ReasoningEffort {
	for _, effort := range ReasoningChoices(providerName, md) {
		if effort == desired {
			return desired
		}
	}
	return sdk.ReasoningDefault
}
