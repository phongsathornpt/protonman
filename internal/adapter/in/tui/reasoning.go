package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/pane"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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
	return m.setReasoningEffort(effort)
}

func (m *bubbleModel) setReasoningEffort(effort sdk.ReasoningEffort) tea.Cmd {
	if effort != sdk.ReasoningDefault {
		profile := m.activeResolvedModelProfile()
		if _, err := profile.ResolveExplicitReasoning(effort); err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
	}
	m.reasoningEffort = effort
	m.agents.SetReasoningEffort(effort)
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
	return pane.ReasoningChoices(profile)
}

func (v *reasoningPaneView) Render(m *bubbleModel) string {
	rows := pane.ReasoningRows(pane.ReasoningSnapshot{
		Height:       m.height,
		Index:        v.index,
		ModelName:    m.activeModel,
		Current:      m.reasoningEffort,
		ModelProfile: m.activeResolvedModelProfile(),
	})
	return renderModalRows(m, accentAssistant, rows)
}

func (v *reasoningPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	choices := reasoningChoices(m.activeResolvedModelProfile())
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	v.index, _, _ = normalizedPickerWindow(v.index, 0, len(choices), len(choices))
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
	return pane.ReasoningEffortDescription(effort)
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
	return pane.ReasoningEffortLabel(effort)
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
