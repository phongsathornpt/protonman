package runtime

import (
	tea "charm.land/bubbletea/v2"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func (m *bubbleModel) permissionView() *permissionPaneView {
	if m.panes.bottom == nil {
		return nil
	}
	view, _ := m.panes.bottom.find(permissionViewID).(*permissionPaneView)
	return view
}

func (m *bubbleModel) hasPermissionView() bool { return m.permissionView() != nil }

func (m *bubbleModel) openPermission(request permissionRequest) {
	if m.panes.bottom == nil {
		return
	}
	m.panes.bottom.push(&permissionPaneView{pending: request})
	if m.activity != "waiting for permission" {
		m.pendingActivity = m.activity
	}
	m.activity = "waiting for permission"
}

func (m *bubbleModel) resolvePermission(option permissionOption) tea.Cmd {
	view := m.permissionView()
	if view == nil {
		return nil
	}
	var resolution permission.Resolution
	var saveCmd tea.Cmd
	request := view.pending.request

	switch option {
	case optionAllowOnce:
		resolution = permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeOnce,
			Reason: "user allowed one call",
		}
	case optionAllowSession:
		resolution = permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeSession,
			Reason: "user allowed this exact request for the session",
		}
	case optionAllowProject:
		resolution = permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeOnce,
			Reason: "user allowed call and saved rule to project",
		}
		if rule, ok := permission.RuleFromRequest(request); ok {
			_ = m.service.AddRule(rule)
			workDir := m.workDir
			saveCmd = func() tea.Msg {
				err := (app.Projects{}).SavePermissionRule(workDir, rule)
				return permissionRuleSavedMsg{scope: "project", rule: rule, err: err}
			}
		}
	case optionAllowGlobal:
		resolution = permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeOnce,
			Reason: "user allowed call and saved rule globally",
		}
		if rule, ok := permission.RuleFromRequest(request); ok {
			_ = m.service.AddRule(rule)
			saveCmd = func() tea.Msg {
				err := (app.UserSettings{}).SavePermissionRule(rule)
				return permissionRuleSavedMsg{scope: "global", rule: rule, err: err}
			}
		}
	default:
		resolution = permission.Resolution{Action: permission.ActionDeny, Reason: "user denied one call"}
	}
	view.pending.response <- permissionResponse{resolution: resolution}
	m.panes.bottom.remove(permissionViewID)
	m.activity = m.pendingActivity
	if m.activity == "" || m.activity == "waiting for permission" {
		m.activity = "running tool"
	}
	m.syncSlashView()
	return saveCmd
}
