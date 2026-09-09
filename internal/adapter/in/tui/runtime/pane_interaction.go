package runtime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type paneActionKind uint8

const (
	paneActionNone paneActionKind = iota
	paneActionClose
	paneActionSetReasoning
	paneActionAcceptSlash
	paneActionToggleSkill
	paneActionReloadProject
	paneActionReloadModels
	paneActionSelectModel
	paneActionOpenProviderSelect
	paneActionOpenProviderEditor
	paneActionPermissionActivity
	paneActionPermissionResolve
	paneActionScrollLines
	paneActionScrollPage
	paneActionProviderDelete
	paneActionProviderActivate
	paneActionProviderModels
	paneActionProviderEdit
)

type paneAction struct {
	kind         paneActionKind
	paneID       string
	reasoning    sdk.ReasoningEffort
	runSlash     bool
	skillName    string
	providerName string
	modelID      string
	activity     string
	permission   permissionOption
	scrollLines  int
	key          tea.KeyPressMsg
	providerItem providerSelectItem
}

type paneKeyResult struct {
	handled     bool
	cmd         tea.Cmd
	action      paneAction
	allowGlobal bool
}

type isolatedPaneKeyHandler interface {
	HandlePaneKey(paneRenderContext, tea.KeyPressMsg) paneKeyResult
}

type modelPaneKeyHandler interface {
	HandleKey(*bubbleModel, tea.KeyPressMsg) (bool, tea.Cmd)
}

func (m *bubbleModel) applyPaneAction(action paneAction) tea.Cmd {
	switch action.kind {
	case paneActionClose:
		m.panes.bottom.remove(action.paneID)
	case paneActionSetReasoning:
		m.panes.bottom.remove(action.paneID)
		return m.setReasoningEffort(action.reasoning)
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
	case paneActionReloadProject:
		if view, _ := m.panes.bottom.find(projectViewID).(*projectPaneView); view != nil {
			return view.reload(m)
		}
	case paneActionReloadModels:
		if view, _ := m.panes.bottom.find(modelSelectViewID).(*modelSelectPaneView); view != nil {
			return view.loadProvider(m, action.runSlash)
		}
	case paneActionSelectModel:
		m.panes.bottom.remove(modelSelectViewID)
		return m.beginModelSelect(action.providerName, action.modelID, false)
	case paneActionOpenProviderSelect:
		m.panes.bottom.remove(modelSelectViewID)
		if !m.panes.bottom.has(providerSelectViewID) {
			m.panes.bottom.push(newProviderSelectPaneView(m))
		}
	case paneActionOpenProviderEditor:
		m.panes.bottom.remove(modelSelectViewID)
		m.panes.bottom.remove(providerSelectViewID)
		if !m.panes.bottom.has(providerViewID) {
			m.pushProviderPane(newProviderPaneView())
		}
	case paneActionPermissionActivity:
		m.activity = action.activity
	case paneActionPermissionResolve:
		return m.resolvePermission(action.permission)
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
		if !m.panes.bottom.has(modelSelectViewID) {
			mv := newModelSelectPaneView(m)
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
	}
	return nil
}
