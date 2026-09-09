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
		if !m.panes.bottom.has(reasoningViewID) {
			m.panes.bottom.push(newReasoningPaneView(m))
		}
		m.requestRelayout()
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
	picker := list.New(items, delegate, maxInt(20, m.layout.width-8), maxInt(6, minInt(18, m.layout.height-4)))
	picker.DisableQuitKeybindings()
	picker.SetFilteringEnabled(false)
	picker.SetShowStatusBar(false)
	// Keep pagination presentation hidden; the list component still owns navigation.
	picker.SetShowPagination(false)
	picker.SetStatusBarItemName("level", "levels")
	picker.Select(selected)
	return &reasoningPaneView{picker: picker, choices: choices}
}

func reasoningChoices(profile modelprofile.Resolved) []sdk.ReasoningEffort {
	return reasoningpane.ReasoningChoices(profile)
}

func (v *reasoningPaneView) Render(ctx paneRenderContext) string {
	v.picker.SetSize(maxInt(20, ctx.width-8), maxInt(6, minInt(18, ctx.height-4)))
	v.picker.Title = "Thinking level"
	if modelName := strings.TrimSpace(ctx.activeModel); modelName != "" {
		v.picker.Title += " · " + modelName
	}
	mode := layoutModeForHeight(ctx.height)
	v.picker.SetShowHelp(mode != layoutTiny)
	delegate := list.NewDefaultDelegate()
	delegate.SetSpacing(0)
	delegate.ShowDescription = mode == layoutNormal
	v.picker.SetDelegate(delegate)
	return renderModalRows(ctx, accentAssistant, strings.Split(v.picker.View(), "\n"))
}

func (v *reasoningPaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	if len(v.choices) == 0 {
		return paneKeyResult{handled: true}
	}
	switch message.String() {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(message.String()[0] - "1"[0])
		if idx >= 0 && idx < len(v.choices) {
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionSetReasoning, paneID: reasoningViewID, reasoning: v.choices[idx]}}
		}
		return paneKeyResult{handled: true}
	case "tab", "shift+tab":
		return paneKeyResult{handled: true}
	case "enter":
		idx := v.picker.Index()
		if idx < 0 || idx >= len(v.choices) {
			return paneKeyResult{handled: true}
		}
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionSetReasoning, paneID: reasoningViewID, reasoning: v.choices[idx]}}
	case "esc", "q":
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: reasoningViewID}}
	case "up", "k", "down", "j", "home", "g", "end", "G", "pgup", "pgdown":
		updated, cmd := v.picker.Update(message)
		v.picker = updated
		return paneKeyResult{handled: true, cmd: cmd}
	default:
		return paneKeyResult{}
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
