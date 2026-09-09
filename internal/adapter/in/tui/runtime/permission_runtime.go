package runtime

import (
	tea "charm.land/bubbletea/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionpolicy"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

const permissionViewID = "permission"

type permissionOption = permissionpolicy.Option

const (
	optionAllowOnce    = permissionpolicy.AllowOnce
	optionAllowSession = permissionpolicy.AllowSession
	optionAllowProject = permissionpolicy.AllowProject
	optionAllowGlobal  = permissionpolicy.AllowGlobal
	optionDeny         = permissionpolicy.Deny
)

type permissionOptionItem = permissionpolicy.Item

func permissionOptionsFor(request permission.Request, projectTrusted bool, hasWorkDir bool) []permissionOptionItem {
	return permissionpolicy.Options(request, projectTrusted, hasWorkDir)
}

func (v *permissionPaneView) options(m *bubbleModel) []permissionOptionItem {
	projectTrusted := false
	hasWorkDir := false
	if m != nil {
		projectTrusted = m.projectTrusted
		hasWorkDir = m.workDir != ""
	}
	return permissionpolicy.Options(v.pending.request, projectTrusted, hasWorkDir)
}

func shortcutHintFor(options []permissionOptionItem) string {
	return permissionpolicy.ShortcutHint(options)
}

type permissionPaneView struct {
	pending permissionRequest
	parked  bool
	index   int
}

func (*permissionPaneView) ID() string             { return permissionViewID }
func (*permissionPaneView) ReplacesComposer() bool { return true }
func (v *permissionPaneView) Render(m *bubbleModel) string {
	return v.card(m)
}

func (v *permissionPaneView) HandleKey(m *bubbleModel, message tea.KeyPressMsg) (bool, tea.Cmd) {
	options := v.options(m)
	if v.index >= len(options) {
		v.index = len(options) - 1
	}
	if v.index < 0 {
		v.index = 0
	}
	if v.parked {
		switch message.String() {
		case "tab":
			v.parked = false
			m.activity = "waiting for permission"
			return true, nil
		case "pgup":
			m.hydrateViewportForScroll()
			m.viewport.PageUp()
			m.followTail = m.viewport.AtBottom()
			return true, nil
		case "pgdown":
			m.viewport.PageDown()
			m.followTail = m.viewport.AtBottom()
			return true, nil
		case "up", "k":
			m.hydrateViewportForScroll()
			m.viewport.ScrollUp(1)
			m.followTail = m.viewport.AtBottom()
			return true, nil
		case "down", "j":
			m.viewport.ScrollDown(1)
			m.followTail = m.viewport.AtBottom()
			return true, nil
		case "y", "s", "p", "g", "n", "1", "2", "3", "4", "5", "enter":
			// Decisions remain available while reviewing the transcript.
		default:
			return !m.matchesGlobalShortcut(message), nil
		}
	}

	switch message.String() {
	case "esc":
		v.parked = true
		m.activity = "permission pending — tab to review"
		return true, nil
	case "up", "k":
		if v.index > 0 {
			v.index--
		}
		return true, nil
	case "down", "j":
		if v.index < len(options)-1 {
			v.index++
		}
		return true, nil
	case "1", "2", "3", "4", "5":
		idx := int(message.String()[0] - '1')
		if idx >= 0 && idx < len(options) {
			return true, m.resolvePermission(options[idx].Option)
		}
		return true, nil
	case "y":
		return true, m.resolvePermission(optionAllowOnce)
	case "s":
		for _, item := range options {
			if item.Option == optionAllowSession {
				return true, m.resolvePermission(optionAllowSession)
			}
		}
		return true, nil
	case "p":
		for _, item := range options {
			if item.Option == optionAllowProject {
				return true, m.resolvePermission(optionAllowProject)
			}
		}
		return true, nil
	case "g":
		for _, item := range options {
			if item.Option == optionAllowGlobal {
				return true, m.resolvePermission(optionAllowGlobal)
			}
		}
		return true, nil
	case "n":
		return true, m.resolvePermission(optionDeny)
	case "enter":
		return true, m.resolvePermission(options[v.index].Option)
	default:
		return !m.matchesGlobalShortcut(message), nil
	}
}

func (m *bubbleModel) permissionView() *permissionPaneView {
	if m.bottom == nil {
		return nil
	}
	view, _ := m.bottom.find(permissionViewID).(*permissionPaneView)
	return view
}

func (m *bubbleModel) hasPermissionView() bool { return m.permissionView() != nil }

func (m *bubbleModel) openPermission(request permissionRequest) {
	if m.bottom == nil {
		return
	}
	m.bottom.push(&permissionPaneView{pending: request})
	if m.activity != "waiting for permission" {
		m.pendingActivity = m.activity
	}
	m.activity = "waiting for permission"
}

func (m *bubbleModel) updatePermission(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	view := m.permissionView()
	if view == nil {
		return m, nil
	}
	_, command := view.HandleKey(m, message)
	return m, command
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
	m.bottom.remove(permissionViewID)
	m.modal = nil
	m.modalParked = false
	m.permIndex = 0
	m.activity = m.pendingActivity
	if m.activity == "" || m.activity == "waiting for permission" {
		m.activity = "running tool"
	}
	m.syncSlashView()
	return saveCmd
}
