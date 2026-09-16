package runtime

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/reasoningpolicy"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/modelprofile"
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func (m *bubbleModel) validateReasoningEffort(effort domain.ReasoningEffort) error {
	if effort == domain.ReasoningDefault {
		return nil
	}
	_, err := m.activeResolvedModelProfile().ResolveExplicitReasoning(effort)
	return err
}

func (m *bubbleModel) reasoningPreferenceValue() domain.ReasoningEffort {
	if m.reasoningPreferenceSet {
		return m.reasoningPreference
	}
	return m.reasoningEffort
}

func (m *bubbleModel) applyReasoningPreference(effort domain.ReasoningEffort, source reasoningPreferenceSource) {
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
			m.appendLine(mutedStyle.Render("  Restored thinking level to " + reasoningpolicy.EffortLabel(desired) + " for " + m.activeModel))
			return true
		}
		m.reasoningCompatibilityFallback = false
		return false
	}
	if m.reasoningCompatibilityFallback && m.reasoningEffort == domain.ReasoningDefault {
		return false
	}
	m.reasoningEffort = domain.ReasoningDefault
	m.reasoningCompatibilityFallback = true
	m.agents.SetReasoningEffort(domain.ReasoningDefault)
	m.appendLine(mutedStyle.Render("  Reset thinking level to auto (requested level " + reasoningpolicy.EffortLabel(desired) + " is unsupported by " + m.activeModel + ")"))
	return true
}

func (m *bubbleModel) activeResolvedModelProfile() modelprofile.Resolved {
	var remote *model.RemoteModel
	if candidate, ok := m.activeRemoteModel(); ok {
		copy := candidate
		remote = &copy
	}
	return model.ResolveModelProfile(m.activeProvider, m.activeModel, remote)
}
