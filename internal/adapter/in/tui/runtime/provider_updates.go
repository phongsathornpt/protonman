package runtime

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func (m *bubbleModel) updateModelsFetched(message modelsFetchedMsg) tea.Cmd {
	if pane := m.panes.bottom.find(providerViewID); pane != nil {
		if pv, ok := pane.(*providerPaneView); ok {
			if message.requestID != pv.fetchRequestID {
				return nil
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
		return nil
	}
	if pane := m.panes.bottom.find(modelSetupViewID); pane != nil {
		if mv, ok := pane.(*modelSetupPaneView); ok {
			currentProvider := mv.activeProviderName()
			if message.requestID != mv.fetchRequestID || !strings.EqualFold(message.providerName, currentProvider) {
				return nil
			}
			mv.fetchCancel = nil
			mv.loading = false
			mv.err = message.err
			if message.err == nil {
				m.modelCatalogs.Set(message.providerName, message.models)
				models := m.modelCatalogs.Models(message.providerName)
				if cfg, configured := m.providers[modelcatalog.NormalizeProviderKey(message.providerName)]; configured {
					models = modelcatalog.VisibleForAccess(message.providerName, cfg.BaseURL, cfg.APIKey, models)
				}
				mv.setModels(models, m.activeProvider, m.activeModel)
				mv.syncReasoningForSelection(mv.reasoningPreference)
			}
			m.requestRelayout()
		}
	}
	return nil
}

func (m *bubbleModel) updateProviderSaved(message providerSavedMsg) tea.Cmd {
	if message.operationID != m.activeProviderSave {
		return nil
	}
	m.activeProviderSave = 0
	if message.err != nil {
		if pane := m.panes.bottom.find(providerViewID); pane != nil {
			if pv, ok := pane.(*providerPaneView); ok {
				pv.state = providerStateSaveError
				pv.errorMessage = message.err.Error()
				m.requestRelayout()
				return nil
			}
		}
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to save provider: %v · run /provider to retry", message.err)))
	} else {
		providerName := strings.TrimSpace(message.providerName)
		providerKey := strings.ToLower(providerName)
		previousKey := strings.ToLower(strings.TrimSpace(message.previousName))
		if previousKey != "" && previousKey != providerKey {
			delete(m.providers, previousKey)
			m.modelCatalogs.Delete(previousKey)
		}
		if m.providers == nil {
			m.providers = make(map[string]config.ProviderConfig)
		}
		m.providers[providerKey] = config.ProviderConfig{Name: providerName, Type: message.providerType, BaseURL: message.baseURL, APIKey: message.apiKey}
		if message.activated {
			m.activeModel = message.modelID
			m.activeProvider = providerName
			m.reconcileReasoningForActiveModel()
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
	m.panes.bottom.remove(providerViewID)
	m.requestRelayout()
	return nil
}

func (m *bubbleModel) updateModelSetupApplied(message modelSetupAppliedMsg) tea.Cmd {
	if message.operationID != m.activeModelSetup {
		return nil
	}
	m.activeModelSetup = 0
	if message.err != nil {
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to set active model: %v · run /model to retry", message.err)))
	} else {
		m.activeModel = message.modelID
		if message.providerName != "" {
			m.activeProvider = message.providerName
		}
		if err := m.validateReasoningEffort(message.reasoning); err == nil {
			m.applyReasoningPreference(message.reasoning, reasoningPreferenceSession)
		} else {
			m.reconcileReasoningForActiveModel()
		}
		m.reconfigureRunner()
		m.appendLine(successStyle.Render(fmt.Sprintf("model → %s · %s", message.modelID, m.activeProvider)))
		if message.unverified {
			m.appendLine(mutedStyle.Render("  Model ID was not present in the discovered catalog; using it as a custom model."))
		}
	}
	m.panes.bottom.remove(modelSetupViewID)
	m.requestRelayout()
	return nil
}

func (m *bubbleModel) updateProviderActiveSelected(message providerActiveSelectedMsg) tea.Cmd {
	if message.operationID != m.activeProviderSelect {
		return nil
	}
	m.activeProviderSelect = 0
	if message.err != nil {
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to switch provider: %v · run /provider to retry", message.err)))
	} else {
		previousModel := m.activeModel
		m.activeProvider = message.providerName
		m.activeModel = message.reconciledModel
		if message.reconciledModel != "" && !strings.EqualFold(previousModel, message.reconciledModel) {
			m.appendLine(mutedStyle.Render(fmt.Sprintf("  Reconciled active model to %s", message.reconciledModel)))
		}
		m.reconcileReasoningForActiveModel()
		m.reconfigureRunner()
		label := "provider → " + message.providerName
		if m.activeModel != "" {
			label += " · " + m.activeModel
		}
		m.appendLine(successStyle.Render(label))
	}
	m.panes.bottom.remove(providerSelectViewID)
	if message.err == nil && m.activeModel == "" {
		return m.openModelSetupPane()
	}
	m.requestRelayout()
	return nil
}

func (m *bubbleModel) updateProviderDeleted(message providerDeletedMsg) tea.Cmd {
	if message.operationID != m.activeProviderDelete {
		return nil
	}
	m.activeProviderDelete = 0
	if message.err != nil {
		m.appendLine(errorStyle.Render(fmt.Sprintf("Failed to remove provider %s: %v · run /provider to retry", message.providerName, message.err)))
	} else {
		delete(m.providers, strings.ToLower(message.providerName))
		m.modelCatalogs.Delete(message.providerName)
		if strings.EqualFold(m.activeProvider, message.providerName) {
			selection, providers := model.ResolvePrimaryModelDefaults(config.ModelConfig{}, m.providers)
			m.providers = providers
			m.activeProvider = selection.Provider
			m.activeModel = selection.Default
			m.reconcileReasoningForActiveModel()
			m.reconfigureRunner()
		}
		label := "provider removed · " + message.providerName
		if m.activeProvider != "" {
			label += " · active " + m.activeProvider
		}
		m.appendLine(successStyle.Render(label))
	}
	m.panes.bottom.remove(providerSelectViewID)
	m.requestRelayout()
	return nil
}
