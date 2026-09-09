package runtime

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func (m *bubbleModel) updateModelsFetched(message modelsFetchedMsg) (tea.Model, tea.Cmd) {
	if pane := m.bottom.find(providerViewID); pane != nil {
		if pv, ok := pane.(*providerPaneView); ok {
			if message.requestID != pv.fetchRequestID {
				return m, nil
			}
			pv.fetchCancel = nil
			if message.err == nil && len(message.models) > 0 {
				m.modelCatalogs.Set(message.providerName, message.models)
			}
			if message.err != nil {
				pv.state = providerStateError
				pv.errorMessage = message.err.Error()
			} else {
				pv.state = providerStateSelectModel
				pv.setFetchedModels(message.models)
			}
			m.requestRelayout()
		}
		return m, nil
	}
	if pane := m.bottom.find(modelSelectViewID); pane != nil {
		if mv, ok := pane.(*modelSelectPaneView); ok {
			currentProvider := mv.activeProviderName()
			if message.requestID != mv.fetchRequestID || !strings.EqualFold(message.providerName, currentProvider) {
				return m, nil
			}
			mv.fetchCancel = nil
			mv.loading = false
			mv.err = message.err
			if message.err == nil {
				m.modelCatalogs.Set(message.providerName, message.models)
				mv.setModels(m.modelCatalogs.Models(message.providerName), m.activeModel)
			}
			m.requestRelayout()
		}
	}
	return m, nil
}

func (m *bubbleModel) updateProviderSaved(message providerSavedMsg) (tea.Model, tea.Cmd) {
	if message.operationID != m.activeProviderSave {
		return m, nil
	}
	m.activeProviderSave = 0
	if message.err != nil {
		if pane := m.bottom.find(providerViewID); pane != nil {
			if pv, ok := pane.(*providerPaneView); ok {
				pv.state = providerStateSaveError
				pv.errorMessage = message.err.Error()
				m.requestRelayout()
				return m, nil
			}
		}
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to save provider: %v", message.err)))
	} else {
		providerName := strings.TrimSpace(message.providerName)
		providerKey := strings.ToLower(providerName)
		previousKey := strings.ToLower(strings.TrimSpace(message.previousName))
		if previousKey != "" && previousKey != providerKey {
			delete(m.providers, previousKey)
		}
		if m.providers == nil {
			m.providers = make(map[string]config.ProviderConfig)
		}
		m.providers[providerKey] = config.ProviderConfig{Name: providerName, Type: message.providerType, BaseURL: message.baseURL, APIKey: message.apiKey}
		if message.activated {
			m.activeModel = message.modelID
			m.activeProvider = providerName
			m.reconfigureRunner()
			label := "provider " + providerName
			if message.modelID != "" {
				label += " · " + message.modelID
			}
			m.appendLine(successStyle.Render(label))
		} else {
			label := "provider " + providerName + " updated"
			if m.activeProvider != "" {
				label += " · active " + m.activeProvider
			}
			m.appendLine(successStyle.Render(label))
		}
	}
	m.bottom.remove(providerViewID)
	m.requestRelayout()
	return m, nil
}

func (m *bubbleModel) updateModelSelected(message modelSelectedMsg) (tea.Model, tea.Cmd) {
	if message.operationID != m.activeModelSelect {
		return m, nil
	}
	m.activeModelSelect = 0
	if message.err != nil {
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to set active model: %v", message.err)))
	} else {
		m.activeModel = message.modelID
		if message.providerName != "" {
			m.activeProvider = message.providerName
		}
		if m.reasoningEffort != sdk.ReasoningDefault {
			profile := m.activeResolvedModelProfile()
			if _, err := profile.ResolveExplicitReasoning(m.reasoningEffort); err != nil {
				previous := m.reasoningEffort
				m.reasoningEffort = sdk.ReasoningDefault
				m.agents.SetReasoningEffort(sdk.ReasoningDefault)
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Reset thinking level to auto (previous level %q is unsupported by %s)", previous, message.modelID)))
			}
		}
		m.reconfigureRunner()
		m.appendLine(successStyle.Render(fmt.Sprintf("model → %s · %s", message.modelID, m.activeProvider)))
		if message.unverified {
			m.appendLine(mutedStyle.Render("  Model ID was not present in the discovered catalog; using it as a custom model."))
		}
	}
	m.bottom.remove(modelSelectViewID)
	m.requestRelayout()
	return m, nil
}

func (m *bubbleModel) updateProviderActiveSelected(message providerActiveSelectedMsg) (tea.Model, tea.Cmd) {
	if message.operationID != m.activeProviderSelect {
		return m, nil
	}
	m.activeProviderSelect = 0
	if message.err != nil {
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to switch provider: %v", message.err)))
	} else {
		m.activeProvider = message.providerName
		if message.reconciledModel != "" {
			m.activeModel = message.reconciledModel
			m.appendLine(mutedStyle.Render(fmt.Sprintf("  Reconciled active model to %s", message.reconciledModel)))
		}
		if m.reasoningEffort != sdk.ReasoningDefault {
			profile := m.activeResolvedModelProfile()
			if _, err := profile.ResolveExplicitReasoning(m.reasoningEffort); err != nil {
				previous := m.reasoningEffort
				m.reasoningEffort = sdk.ReasoningDefault
				m.agents.SetReasoningEffort(sdk.ReasoningDefault)
				m.appendLine(mutedStyle.Render(fmt.Sprintf("  Reset thinking level to auto (previous level %q is unsupported by %s)", previous, m.activeModel)))
			}
		}
		m.reconfigureRunner()
		label := "provider → " + message.providerName
		if m.activeModel != "" {
			label += " · " + m.activeModel
		}
		m.appendLine(successStyle.Render(label))
	}
	m.bottom.remove(providerSelectViewID)
	m.requestRelayout()
	return m, nil
}

func (m *bubbleModel) updateProviderDeleted(message providerDeletedMsg) (tea.Model, tea.Cmd) {
	if message.operationID != m.activeProviderDelete {
		return m, nil
	}
	m.activeProviderDelete = 0
	if message.err != nil {
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to remove provider %s: %v", message.providerName, message.err)))
	} else {
		delete(m.providers, strings.ToLower(message.providerName))
		m.modelCatalogs.Delete(message.providerName)
		if strings.EqualFold(m.activeProvider, message.providerName) {
			m.activeProvider = ""
			if len(m.providers) > 0 {
				keys := make([]string, 0, len(m.providers))
				for k := range m.providers {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				m.activeProvider = keys[0]
				models := m.modelCatalogs.Models(m.activeProvider)
				if len(models) > 0 {
					m.activeModel = models[0].ID
				} else {
					m.activeModel = ""
				}
				m.reconfigureRunner()
			} else {
				m.activeModel = ""
				m.runner = nil
				m.bottom.setHasRunner(false)
			}
		}
		label := "provider removed · " + message.providerName
		if m.activeProvider != "" {
			label += " · active " + m.activeProvider
		}
		m.appendLine(successStyle.Render(label))
	}
	m.bottom.remove(providerSelectViewID)
	m.requestRelayout()
	return m, nil
}
