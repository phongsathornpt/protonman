package runtime

import (
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
)

type paneAction struct {
	kind      paneActionKind
	paneID    string
	reasoning sdk.ReasoningEffort
	runSlash  bool
	skillName string
}

type paneKeyResult struct {
	handled bool
	cmd     tea.Cmd
	action  paneAction
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
	}
	return nil
}
