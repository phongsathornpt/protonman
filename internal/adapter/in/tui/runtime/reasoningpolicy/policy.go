package reasoningpolicy

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func Choices(profile modelprofile.Resolved) []sdk.ReasoningEffort {
	choices := []sdk.ReasoningEffort{sdk.ReasoningDefault}
	for _, level := range profile.Reasoning.Levels {
		if level != sdk.ReasoningDefault {
			choices = append(choices, level)
		}
	}
	return choices
}

func EffortLabel(effort sdk.ReasoningEffort) string {
	if effort == sdk.ReasoningDefault {
		return "auto"
	}
	return string(effort)
}

func Summary(providerName string, md model.RemoteModel, includeDefault bool) string {
	profile := model.ResolveModelProfile(providerName, md.ID, &md)
	supported, known := profile.Reasoning.Support.Bool()
	if !known || !supported {
		return ""
	}
	if len(profile.Reasoning.Levels) == 0 {
		return "reasoning"
	}
	levels := make([]string, 0, len(profile.Reasoning.Levels))
	for _, level := range profile.Reasoning.Levels {
		levels = append(levels, string(level))
	}
	summary := "reasoning " + strings.Join(levels, "/")
	if includeDefault && profile.Reasoning.Default != sdk.ReasoningDefault {
		summary += " (default " + string(profile.Reasoning.Default) + ")"
	}
	return summary
}
