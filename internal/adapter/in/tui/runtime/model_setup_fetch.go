package runtime

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/modelcatalog"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

func (v *modelSetupPaneView) beginFetch(parent context.Context, providerName string, cfg config.ProviderConfig, timeouts ...time.Duration) tea.Cmd {
	discoveryTimeout := runtimepolicy.ModelDiscoveryTimeout
	if len(timeouts) > 0 && timeouts[0] > 0 {
		discoveryTimeout = timeouts[0]
	}
	v.cancelFetch()
	if parent == nil {
		v.fetchRequestID = 0
		v.loading = false
		v.err = errMissingRuntimeContext
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	v.fetchCancel = cancel
	v.fetchRequestID = nextAsyncOperationID()
	v.loading = true
	v.err = nil
	v.models = nil
	v.allModels = nil
	v.picker.GoToStart()
	return fetchProviderModelsCmd(providerFetchRequest{ctx: ctx, requestID: v.fetchRequestID, providerName: providerName, providerType: cfg.Type, baseURL: cfg.BaseURL, apiKey: cfg.APIKey, discoveryTimeout: discoveryTimeout})
}

func (v *modelSetupPaneView) cancelFetch() {
	if v == nil || v.fetchCancel == nil {
		return
	}
	v.fetchCancel()
	v.fetchCancel = nil
}

func (v *modelSetupPaneView) loadProvider(m *bubbleModel, force bool) tea.Cmd {
	if v == nil || m == nil {
		return nil
	}
	providerName := v.activeProviderName()
	v.cancelFetch()
	v.loading = false
	v.err = nil
	if !force {
		if models, ok := m.modelCatalogs.FreshModels(providerName, time.Now(), m.runtimeConfig.ModelCatalogTTL); ok {
			if cfg, configured := m.providers[modelcatalog.NormalizeProviderKey(providerName)]; configured {
				models = visibleModelsForAccess(providerName, cfg.BaseURL, cfg.APIKey, models)
			}
			v.setModels(models, m.activeProvider, m.activeModel)
			v.syncReasoningForSelection(v.reasoningPreference)
			return nil
		}
	}
	cfg, configured := m.providers[modelcatalog.NormalizeProviderKey(providerName)]
	if configured {
		if model.ProviderHasUsableAuth(providerName, cfg.BaseURL, cfg.APIKey) {
			return v.beginFetch(m.ctx, providerName, cfg, m.runtimeConfig.ModelDiscoveryTimeout)
		}
	}
	v.setModels(nil, m.activeProvider, m.activeModel)
	v.syncReasoningForSelection(v.reasoningPreference)
	return nil
}

func (m *bubbleModel) openModelSetupPane() tea.Cmd {
	if m == nil || m.panes.bottom.has(modelSetupViewID) {
		return nil
	}
	view := newModelSetupPaneView(m)
	m.panes.bottom.push(view)
	m.requestRelayout()
	return view.loadProvider(m, false)
}

func (m *bubbleModel) toggleModelSetupPane() tea.Cmd {
	if m.panes.bottom.has(modelSetupViewID) {
		m.panes.bottom.remove(modelSetupViewID)
		m.requestRelayout()
		return nil
	}
	return m.openModelSetupPane()
}
