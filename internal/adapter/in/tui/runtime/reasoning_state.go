package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func (m *bubbleModel) handleReasoningCommand(argument string) tea.Cmd {
	argument = strings.TrimSpace(argument)
	if argument == "" {
		return m.openModelSetupPane()
	}
	effort, err := sdk.ParseReasoningEffort(argument)
	if err != nil {
		m.appendError("invalid reasoning effort: use auto, none, minimal, low, medium, high, xhigh, or max")
		m.refreshViewport()
		return nil
	}
	return m.setReasoningEffort(effort)
}

func (m *bubbleModel) validateReasoningEffort(effort sdk.ReasoningEffort) error {
	if effort == sdk.ReasoningDefault {
		return nil
	}
	_, err := m.activeResolvedModelProfile().ResolveExplicitReasoning(effort)
	return err
}

func (m *bubbleModel) reasoningPreferenceValue() sdk.ReasoningEffort {
	if m.reasoningPreferenceSet {
		return m.reasoningPreference
	}
	return m.reasoningEffort
}

func (m *bubbleModel) applyReasoningPreference(effort sdk.ReasoningEffort, source reasoningPreferenceSource) {
	m.reasoningPreference = effort
	m.reasoningPreferenceSet = true
	m.reasoningPreferenceSource = source
	m.reasoningCompatibilityFallback = false
	m.reasoningEffort = effort
	m.agents.SetReasoningEffort(effort)
}

func (m *bubbleModel) reconcileReasoningForActiveModel() bool {
	if !m.reasoningPreferenceSet {
		m.reasoningPreference = m.reasoningEffort
		m.reasoningPreferenceSet = true
		m.reasoningPreferenceSource = reasoningPreferenceConfig
	}
	desired := m.reasoningPreference
	if err := m.validateReasoningEffort(desired); err == nil {
		if m.reasoningCompatibilityFallback && m.reasoningEffort != desired {
			m.reasoningEffort = desired
			m.reasoningCompatibilityFallback = false
			m.agents.SetReasoningEffort(desired)
			m.appendLine(mutedStyle.Render("  Restored thinking level to " + reasoningEffortLabel(desired) + " for " + m.activeModel))
			return true
		}
		m.reasoningCompatibilityFallback = false
		return false
	}
	if m.reasoningCompatibilityFallback && m.reasoningEffort == sdk.ReasoningDefault {
		return false
	}
	m.reasoningEffort = sdk.ReasoningDefault
	m.reasoningCompatibilityFallback = true
	m.agents.SetReasoningEffort(sdk.ReasoningDefault)
	m.appendLine(mutedStyle.Render("  Reset thinking level to auto (requested level " + reasoningEffortLabel(desired) + " is unsupported by " + m.activeModel + ")"))
	return true
}

func (m *bubbleModel) setReasoningEffort(effort sdk.ReasoningEffort) tea.Cmd {
	if err := m.validateReasoningEffort(effort); err != nil {
		m.appendError(err.Error())
		m.refreshViewport()
		return nil
	}
	m.applyReasoningPreference(effort, reasoningPreferenceSession)
	m.reconfigureRunner()
	m.appendLine(successStyle.Render("Thinking level set to " + reasoningEffortLabel(effort) + " for this session."))
	m.refreshViewport()
	return nil
}

func reasoningChoices(profile modelprofile.Resolved) []sdk.ReasoningEffort {
	choices := []sdk.ReasoningEffort{sdk.ReasoningDefault}
	if len(profile.Reasoning.Levels) > 0 {
		for _, level := range profile.Reasoning.Levels {
			if level != sdk.ReasoningDefault {
				choices = append(choices, level)
			}
		}
		return choices
	}
	// Unknown model metadata must not fabricate portable effort levels. The
	// catalog or a known family profile is the authority for selectable levels;
	// otherwise only provider/model default (auto) is safe to expose.
	return choices
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
