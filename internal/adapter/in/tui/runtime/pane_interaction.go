package runtime

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
	tododomain "github.com/phongsathornpt/protonman/internal/feature/todo"
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
	paneActionResumeSession
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
	sessionID      string
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

type isolatedPanePasteHandler interface {
	HandlePanePaste(paneRenderContext, tea.PasteMsg) paneKeyResult
}

type isolatedPaneMsgHandler interface {
	HandlePaneMsg(paneRenderContext, tea.Msg) paneKeyResult
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
		active, err := m.skills.Toggle(action.skillName)
		if err != nil {
			m.appendError(err.Error())
			m.refreshViewport()
			return nil
		}
		m.persistActiveSkills()
		state := "Deactivated"
		if active {
			state = "Activated"
		}
		noticeCmd := m.showTransientNotice(fmt.Sprintf("%s skill %q", state, action.skillName))
		if view, _ := m.panes.bottom.find(skillsViewID).(*skillsPaneView); view != nil {
			refreshCmd := view.refreshItems(newPaneRenderContext(m))
			return tea.Batch(noticeCmd, refreshCmd)
		}
		return noticeCmd
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
	case paneActionResumeSession:
		m.panes.bottom.remove(sessionResumeViewID)
		return m.resumeSession(action.sessionID)
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
			return view.beginFetch(m.ctx, m.application.Models, m.runtimeConfig.ModelDiscoveryTimeout)
		}
	case paneActionProviderSave:
		return m.beginProviderSave(action.providerSave)
	}
	return nil
}

type paneRenderContext struct {
	width              int
	height             int
	activeModel        string
	spinner            string
	projectTrusted     bool
	hasWorkDir         bool
	workDir            string
	skillItems         []skillListItem
	todos              []tododomain.Item
	slashMatches       []slashCommand
	agentSnapshot      []agent.AgentStatus
	agentActivity      map[string]string
	subagentsEnabled   bool
	keyboardCapability keyboardCapability
	providers          map[string]config.ProviderConfig
}

func newPaneRenderContext(m *bubbleModel) paneRenderContext {
	ctx := paneRenderContext{width: defaultBubbleWidth, height: defaultBubbleHeight}
	if m == nil {
		return ctx
	}
	ctx.width = m.layout.width
	ctx.height = m.layout.height
	ctx.activeModel = m.activeModel
	ctx.spinner = m.spinnerIndicator()
	ctx.projectTrusted = m.projectTrusted
	ctx.hasWorkDir = m.workDir != ""
	ctx.workDir = m.workDir
	ctx.todos = tododomain.CloneItems(m.todo)
	ctx.slashMatches = append([]slashCommand(nil), m.slashMatches()...)
	ctx.agentSnapshot = append([]agent.AgentStatus(nil), m.agentSnapshot...)
	ctx.subagentsEnabled = m.subagentsEnabled
	ctx.keyboardCapability = m.keyboardCapability
	ctx.providers = make(map[string]config.ProviderConfig, len(m.providers))
	for name, cfg := range m.providers {
		ctx.providers[name] = cfg
	}
	ctx.agentActivity = make(map[string]string, len(m.agentActivity))
	for id, state := range m.agentActivity {
		ctx.agentActivity[id] = state.String()
	}
	if m.skills != nil {
		for _, item := range m.skills.List() {
			ctx.skillItems = append(ctx.skillItems, skillListItem{
				name:        item.Name,
				description: item.Description,
				scope:       string(item.Scope),
				active:      m.skills.IsActivated(item.Name),
			})
		}
	}

	return ctx
}
