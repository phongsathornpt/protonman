package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/modelprofile"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

const reasoningViewID = "reasoning"

type reasoningPaneView struct {
	index int
}

func (*reasoningPaneView) ID() string             { return reasoningViewID }
func (*reasoningPaneView) ReplacesComposer() bool { return true }

func (m *bubbleModel) handleReasoningCommand(argument string) tea.Cmd {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		if !m.bottom.has(reasoningViewID) {
			m.bottom.push(newReasoningPaneView(m))
		}
		m.relayout()
		return nil
	}

	effort, err := sdk.ParseReasoningEffort(argument)
	if err != nil {
		m.appendError("invalid reasoning effort: use auto, none, low, medium, high, xhigh, or max")
		m.refreshViewport()
		return nil
	}
	if effort != sdk.ReasoningDefault {
		profile := m.activeResolvedModelProfile()
		if _, err := profile.ResolveExplicitReasoning(effort); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
	}

	return m.setReasoningEffort(effort)
}

func (m *bubbleModel) setReasoningEffort(effort sdk.ReasoningEffort) tea.Cmd {
	m.reasoningEffort = effort
	if m.coordinator != nil {
		m.coordinator.SetReasoningEffort(effort)
	}
	m.reconfigureRunner()
	m.appendLine(successStyle.Render("Thinking level set to " + reasoningEffortLabel(effort) + "."))
	m.refreshViewport()
	return nil
}

func newReasoningPaneView(m *bubbleModel) *reasoningPaneView {
	view := &reasoningPaneView{}
	choices := reasoningChoices(m.activeResolvedModelProfile())
	for i, effort := range choices {
		if effort == m.reasoningEffort {
			view.index = i
			break
		}
	}
	return view
}

func reasoningChoices(profile modelprofile.Resolved) []sdk.ReasoningEffort {
	choices := []sdk.ReasoningEffort{sdk.ReasoningDefault}
	for _, level := range profile.Reasoning.Levels {
		if level != sdk.ReasoningDefault {
			choices = append(choices, level)
		}
	}
	return choices
}

func (v *reasoningPaneView) Render(m *bubbleModel) string {
	profile := m.activeResolvedModelProfile()
	choices := reasoningChoices(profile)
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	if v.index < 0 || v.index >= len(choices) {
		v.index = 0
	}

	modelName := strings.TrimSpace(m.activeModel)
	if modelName == "" {
		modelName = "current model"
	}
	rows := []string{
		brandStyle.Render("Thinking level"),
		mutedStyle.Render(modelName + " · choose how much reasoning to use"),
		"",
	}
	for i, effort := range choices {
		cursor := "  "
		if i == v.index {
			cursor = glyphPrompt
		}
		label := reasoningEffortLabel(effort)
		detail := reasoningEffortDescription(effort)
		badges := make([]string, 0, 2)
		if effort == m.reasoningEffort {
			badges = append(badges, "current")
		}
		if effort != sdk.ReasoningDefault && effort == profile.Reasoning.Default {
			badges = append(badges, "model default")
		}
		if len(badges) > 0 {
			detail += " · " + strings.Join(badges, " · ")
		}
		line := fmt.Sprintf("%s%-7s %s", cursor, label, mutedStyle.Render(detail))
		if i == v.index {
			line = fmt.Sprintf("%s%s %s", cursor, brandStyle.Bold(true).Render(label), mutedStyle.Render(detail))
		}
		rows = append(rows, line)
	}
	if len(profile.Reasoning.Levels) == 0 {
		rows = append(rows, "", mutedStyle.Render("This model does not publish selectable thinking levels."))
	}
	rows = append(rows, "", mutedStyle.Render("↑/↓ move · enter select · esc close · /reasoning <level> also works"))
	if layoutModeForHeight(m.height) == layoutTiny {
		rows = compactPickerRows(rows)
	}
	return renderModalRows(m, accentAssistant, rows)
}

func (v *reasoningPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	choices := reasoningChoices(m.activeResolvedModelProfile())
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	switch message.String() {
	case "up", "k":
		v.index = (v.index - 1 + len(choices)) % len(choices)
		return true, nil
	case "down", "j":
		v.index = (v.index + 1) % len(choices)
		return true, nil
	case "home", "g":
		v.index = 0
		return true, nil
	case "end", "G":
		v.index = len(choices) - 1
		return true, nil
	case "enter":
		effort := choices[v.index]
		m.bottom.remove(reasoningViewID)
		return true, m.setReasoningEffort(effort)
	case "esc", "q":
		m.bottom.remove(reasoningViewID)
		return true, nil
	default:
		return false, nil
	}
}

func reasoningEffortDescription(effort sdk.ReasoningEffort) string {
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

func (m *bubbleModel) activeResolvedModelProfile() modelprofile.Resolved {
	var remote *model.RemoteModel
	if candidate, ok := m.activeRemoteModel(); ok {
		copy := candidate
		remote = &copy
	}
	return model.ResolveModelProfile(m.activeProvider, m.activeModel, remote)
}

func reasoningEffortLabel(effort sdk.ReasoningEffort) string {
	if effort == sdk.ReasoningDefault {
		return "auto"
	}
	return string(effort)
}

func remoteModelReasoningSummary(providerName string, md model.RemoteModel, includeDefault bool) string {
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
