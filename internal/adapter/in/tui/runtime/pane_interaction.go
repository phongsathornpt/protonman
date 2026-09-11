package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type paneActionKind uint8

const (
	paneActionNone paneActionKind = iota
	paneActionClose
	paneActionAcceptSlash
	paneActionToggleSkill
	paneActionReloadModels
	paneActionApplyModelSetup
	paneActionOpenProviderSelect
	paneActionOpenProviderEditor
	paneActionPermissionActivity
	paneActionPermissionResolve
	paneActionSetPermissionMode
	paneActionSetLowConcurrency
	paneActionScrollLines
	paneActionScrollPage
	paneActionProviderDelete
	paneActionProviderActivate
	paneActionProviderModels
	paneActionProviderEdit
	paneActionProviderFetch
	paneActionProviderSave
)

type paneAction struct {
	kind           paneActionKind
	paneID         string
	reasoning      sdk.ReasoningEffort
	runSlash       bool
	skillName      string
	providerName   string
	modelID        string
	activity       string
	permission     permissionOption
	permissionMode permissionModeChoice
	lowConcurrency model.LowConcurrencySetting
	scrollLines    int
	key            tea.KeyPressMsg
	providerItem   providerSelectItem
	providerSave   providerSaveRequest
}

type paneKeyResult struct {
	handled bool
	cmd     tea.Cmd
	action  paneAction
}

type isolatedPaneKeyHandler interface {
	HandlePaneKey(paneRenderContext, tea.KeyPressMsg) paneKeyResult
}

func (m *bubbleModel) applyPaneAction(action paneAction) tea.Cmd {
	switch action.kind {
	case paneActionClose:
		m.panes.bottom.remove(action.paneID)
	case paneActionAcceptSlash:
		_, cmd := m.acceptSlash(action.runSlash)
		return cmd
	case paneActionToggleSkill:
		if m.skills == nil {
			return nil
		}
		_, _ = m.skills.Toggle(action.skillName)
		if view, _ := m.panes.bottom.find(skillsViewID).(*skillsPaneView); view != nil {
			return view.refreshItems(newPaneRenderContext(m))
		}
	case paneActionReloadModels:
		if view, _ := m.panes.bottom.find(modelSetupViewID).(*modelSetupPaneView); view != nil {
			return view.loadProvider(m, action.runSlash)
		}
	case paneActionApplyModelSetup:
		m.panes.bottom.remove(modelSetupViewID)
		return m.beginModelSetupSelect(action.providerName, action.modelID, action.reasoning, false)
	case paneActionOpenProviderSelect:
		m.panes.bottom.remove(modelSetupViewID)
		if !m.panes.bottom.has(providerSelectViewID) {
			m.panes.bottom.push(newProviderSelectPaneView(m))
		}
	case paneActionOpenProviderEditor:
		m.panes.bottom.remove(modelSetupViewID)
		m.panes.bottom.remove(providerSelectViewID)
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneView())
		}
	case paneActionPermissionActivity:
		m.activity = action.activity
	case paneActionPermissionResolve:
		return m.resolvePermission(action.permission)
	case paneActionSetPermissionMode:
		m.panes.bottom.remove(permissionModeViewID)
		m.applyPermissionModeChoice(action.permissionMode)
	case paneActionSetLowConcurrency:
		m.panes.bottom.remove(lowConcurrencyViewID)
		return m.applyLowConcurrencySetting(action.lowConcurrency)
	case paneActionScrollLines:
		m.scrollConversationLines(action.scrollLines)
	case paneActionScrollPage:
		return m.updateConversationViewport(action.key)
	case paneActionProviderDelete:
		m.panes.bottom.remove(providerSelectViewID)
		return m.beginProviderDelete(action.providerItem.name)
	case paneActionProviderActivate:
		m.panes.bottom.remove(providerSelectViewID)
		return m.beginProviderSelect(action.providerItem.name)
	case paneActionProviderModels:
		m.panes.bottom.remove(providerSelectViewID)
		if !m.panes.bottom.has(modelSetupViewID) {
			mv := newModelSetupPaneView(m)
			for i, name := range mv.providerNames {
				if strings.EqualFold(name, action.providerItem.name) {
					mv.providerIndex = i
					break
				}
			}
			m.panes.bottom.push(mv)
			if _, ok := m.providers[strings.ToLower(action.providerItem.name)]; ok {
				return mv.loadProvider(m, false)
			}
		}
	case paneActionProviderEdit:
		m.panes.bottom.remove(providerSelectViewID)
		if !m.panes.bottom.has(providerViewID) {
			item := action.providerItem
			if item.isConfigured {
				if cfg, ok := m.providers[strings.ToLower(item.name)]; ok {
					pv := newProviderPaneViewWithConfig(cfg)
					pv.activateOnSave = item.isActive
					m.pushProviderPane(pv)
				} else {
					pv := newProviderPaneViewWithPreset(item.name)
					pv.activateOnSave = item.isActive
					m.pushProviderPane(pv)
				}
			} else if item.kind == providerItemPreset {
				m.pushProviderPane(newProviderPaneViewWithPreset(item.presetID))
			} else {
				m.pushProviderPane(newProviderPaneView())
			}
		}
	case paneActionProviderFetch:
		if view, _ := m.panes.bottom.find(providerViewID).(*providerPaneView); view != nil {
			return view.beginFetch(m.ctx, m.runtimeConfig.ModelDiscoveryTimeout)
		}
	case paneActionProviderSave:
		return m.beginProviderSave(action.providerSave)
	}
	return nil
}
