package runtime

import (
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	reasoningpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/reasoning"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const reasoningViewID = "reasoning"

type reasoningListItem struct {
	effort  sdk.ReasoningEffort
	current bool
}

func (i reasoningListItem) FilterValue() string { return reasoningEffortLabel(i.effort) }
func (i reasoningListItem) Title() string {
	label := reasoningEffortLabel(i.effort)
	if i.current {
		return "✓ " + label
	}
	return label
}
func (i reasoningListItem) Description() string { return reasoningEffortDescription(i.effort) }

type reasoningPaneView struct {
	picker  list.Model
	choices []sdk.ReasoningEffort
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
	m.appendLine(successStyle.Render("Thinking level set to " + reasoningEffortLabel(effort) + " for this session."))
	m.refreshViewport()
	return nil
}

func newReasoningPaneView(m *bubbleModel) *reasoningPaneView {
	choices := reasoningChoices(m.activeResolvedModelProfile())
	if len(choices) == 0 {
		choices = []sdk.ReasoningEffort{sdk.ReasoningDefault}
	}
	items := make([]list.Item, 0, len(choices))
	selected := 0
	for i, effort := range choices {
		current := effort == m.reasoningEffort
		items = append(items, reasoningListItem{effort: effort, current: current})
		if current {
			selected = i
		}
	}
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	picker := list.New(items, delegate, maxInt(20, m.width-8), maxInt(6, minInt(18, m.height-4)))
	picker.DisableQuitKeybindings()
	picker.SetFilteringEnabled(false)
	picker.SetShowStatusBar(false)
	picker.SetShowPagination(false)
	picker.SetStatusBarItemName("level", "levels")
	picker.Select(selected)
	return &reasoningPaneView{picker: picker, choices: choices}
}

func reasoningChoices(profile modelprofile.Resolved) []sdk.ReasoningEffort {
	return reasoningpane.ReasoningChoices(profile)
}

func (v *reasoningPaneView) Render(m *bubbleModel) string {
	v.picker.SetSize(maxInt(20, m.width-8), maxInt(6, minInt(18, m.height-4)))
	v.picker.Title = "Thinking level"
	if modelName := strings.TrimSpace(m.activeModel); modelName != "" {
		v.picker.Title += " · " + modelName
	}
	mode := layoutModeForHeight(m.height)
	v.picker.SetShowHelp(mode != layoutTiny)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)
	return renderModalRows(m, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *reasoningPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	if len(v.choices) == 0 {
		return true, nil
	}
	switch message.String() {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(message.String()[0] - '1')
		if idx >= 0 && idx < len(v.choices) {
			m.bottom.remove(reasoningViewID)
			return true, m.setReasoningEffort(v.choices[idx])
		}
		return true, nil
	case "tab", "shift+tab":
		return true, nil
	case "enter":
		idx := v.picker.Index()
		if idx < 0 || idx >= len(v.choices) {
			return true, nil
		}
		m.bottom.remove(reasoningViewID)
		return true, m.setReasoningEffort(v.choices[idx])
	case "esc", "q":
		m.bottom.remove(reasoningViewID)
		return true, nil
	case "up", "k", "down", "j", "home", "g", "end", "G", "pgup", "pgdown":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return true, cmd
	default:
		return false, nil
	}
}

func reasoningEffortDescription(effort sdk.ReasoningEffort) string {
	return reasoningpane.ReasoningEffortDescription(effort)
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
	return reasoningpane.ReasoningEffortLabel(effort)
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
