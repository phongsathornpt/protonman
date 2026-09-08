package pane

import (
	"fmt"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/textview"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type ReasoningSnapshot struct {
	Height       int
	Index        int
	ModelName    string
	Current      sdk.ReasoningEffort
	ModelProfile modelprofile.Resolved
}

func ReasoningChoices(profile modelprofile.Resolved) []sdk.ReasoningEffort {
	choices := []sdk.ReasoningEffort{sdk.ReasoningDefault}
	if len(profile.Reasoning.Levels) > 0 {
		for _, level := range profile.Reasoning.Levels {
			if level != sdk.ReasoningDefault {
				choices = append(choices, level)
			}
		}
		return choices
	}
	if profile.Reasoning.Support == modelprofile.SupportUnknown {
		return []sdk.ReasoningEffort{
			sdk.ReasoningDefault,
			sdk.ReasoningLow,
			sdk.ReasoningMedium,
			sdk.ReasoningHigh,
		}
	}
	return choices
}

func ReasoningRows(snapshot ReasoningSnapshot) []string {
	choices := ReasoningChoices(snapshot.ModelProfile)
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	index := snapshot.Index
	if index < 0 {
		index = 0
	}
	if index >= len(choices) {
		index = len(choices) - 1
	}
	modelName := strings.TrimSpace(snapshot.ModelName)
	if modelName == "" {
		modelName = "current model"
	}
	rows := []string{
		tuistyle.BrandStyle.Render("Thinking level"),
		tuistyle.MutedStyle.Render(modelName + " · choose how much reasoning to use"),
		"",
	}
	for i, effort := range choices {
		cursor := "  "
		if i == index {
			cursor = tuistyle.GlyphPrompt
		}
		label := ReasoningEffortLabel(effort)
		detail := ReasoningEffortDescription(effort)
		if effort == sdk.ReasoningDefault {
			if snapshot.ModelProfile.Reasoning.Support == modelprofile.SupportNo {
				detail = "recommended; extended reasoning not supported by this model"
			} else if snapshot.ModelProfile.Reasoning.Default != sdk.ReasoningDefault && snapshot.ModelProfile.Reasoning.Default != "" {
				detail = fmt.Sprintf("recommended; follow defaults (model resolves to: %s)", snapshot.ModelProfile.Reasoning.Default)
			}
		}
		badges := make([]string, 0, 2)
		if effort == snapshot.Current {
			badges = append(badges, "current")
		}
		if effort != sdk.ReasoningDefault && effort == snapshot.ModelProfile.Reasoning.Default {
			badges = append(badges, "model default")
		}
		if len(badges) > 0 {
			detail += " · " + strings.Join(badges, " · ")
		}
		line := cursor + textview.PadRight(label, 7) + " " + tuistyle.MutedStyle.Render(detail)
		if i == index {
			line = fmt.Sprintf("%s%s %s", cursor, tuistyle.BrandStyle.Bold(true).Render(label), tuistyle.MutedStyle.Render(detail))
		}
		rows = append(rows, line)
	}
	if len(snapshot.ModelProfile.Reasoning.Levels) == 0 && snapshot.ModelProfile.Reasoning.Support != modelprofile.SupportUnknown {
		rows = append(rows, "", tuistyle.MutedStyle.Render("This model does not publish selectable thinking levels."))
	}
	rows = append(rows, "", tuistyle.MutedStyle.Render("1-5 select · ↑/↓ move · enter select · esc close · /reasoning <level>"))
	if ModeForHeight(snapshot.Height) == LayoutTiny {
		rows = CompactRows(rows)
	}
	return rows
}

func ReasoningEffortDescription(effort sdk.ReasoningEffort) string {
	switch effort {
	case sdk.ReasoningDefault:
		return "recommended; follow agent and model defaults"
	case sdk.ReasoningNone:
		return "fastest; disable extra reasoning"
	case sdk.ReasoningLow:
		return "fast; light reasoning"
	case sdk.ReasoningMedium:
		return "balanced speed and depth"
	case sdk.ReasoningHigh:
		return "deeper reasoning for harder tasks"
	case sdk.ReasoningXHigh:
		return "very deep reasoning"
	case sdk.ReasoningMax:
		return "maximum provider-supported reasoning"
	default:
		return ""
	}
}

func ReasoningEffortLabel(effort sdk.ReasoningEffort) string {
	if effort == sdk.ReasoningDefault {
		return "auto"
	}
	return string(effort)
}
