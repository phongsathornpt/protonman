package turn

import (
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func (l *Loop) resolveReasoningPolicy() (modelprofile.ReasoningResolution, error) {
	requested := l.reasoningEffort
	if requested == sdk.ReasoningDefault {
		return modelprofile.ReasoningResolution{Source: modelprofile.ReasoningSourceProviderDefault}, nil
	}
	if profile, ok := model.ResolvedModelProfile(l.languageModel); ok {
		return profile.ResolveReasoning(requested, l.reasoningExplicit)
	}
	if l.reasoningExplicit {
		return modelprofile.ReasoningResolution{
			Requested: requested, Effective: requested, Source: modelprofile.ReasoningSourceExplicit,
		}, nil
	}
	return modelprofile.ReasoningResolution{
		Requested: requested, Source: modelprofile.ReasoningSourceProviderDefault,
	}, nil
}

func reasoningRequestedLabel(resolution modelprofile.ReasoningResolution) string {
	if resolution.Requested == sdk.ReasoningDefault {
		return "auto"
	}
	return string(resolution.Requested)
}

func reasoningEffectiveLabel(resolution modelprofile.ReasoningResolution) string {
	if resolution.Effective == sdk.ReasoningDefault {
		return "provider_default"
	}
	return string(resolution.Effective)
}
