package runtime

import (
	tea "charm.land/bubbletea/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionpolicy"
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
func (v *permissionPaneView) Render(ctx paneRenderContext) string {
	return v.card(ctx)
}

func (v *permissionPaneView) HandlePaneKey(ctx paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	options := permissionpolicy.Options(v.pending.request, ctx.projectTrusted, ctx.hasWorkDir)
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
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionPermissionActivity, activity: "waiting for permission"}}
		case "pgup", "pgdown":
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionScrollPage, key: message}}
		case "up", "k":
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionScrollLines, scrollLines: -1}}
		case "down", "j":
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionScrollLines, scrollLines: 1}}
		case "y", "s", "p", "g", "n", "1", "2", "3", "4", "5", "enter":
			// Decisions remain available while reviewing the transcript.
		default:
			return paneKeyResult{handled: true, allowGlobal: true}
		}
	}

	resolve := func(option permissionOption) paneKeyResult {
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionPermissionResolve, permission: option}}
	}
	switch message.String() {
	case "esc":
		v.parked = true
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionPermissionActivity, activity: "permission pending — tab to review"}}
	case "up", "k":
		if v.index > 0 {
			v.index--
		}
		return paneKeyResult{handled: true}
	case "down", "j":
		if v.index < len(options)-1 {
			v.index++
		}
		return paneKeyResult{handled: true}
	case "1", "2", "3", "4", "5":
		idx := int(message.String()[0] - '1')
		if idx >= 0 && idx < len(options) {
			return resolve(options[idx].Option)
		}
		return paneKeyResult{handled: true}
	case "y":
		return resolve(optionAllowOnce)
	case "s":
		for _, item := range options {
			if item.Option == optionAllowSession {
				return resolve(optionAllowSession)
			}
		}
		return paneKeyResult{handled: true}
	case "p":
		for _, item := range options {
			if item.Option == optionAllowProject {
				return resolve(optionAllowProject)
			}
		}
		return paneKeyResult{handled: true}
	case "g":
		for _, item := range options {
			if item.Option == optionAllowGlobal {
				return resolve(optionAllowGlobal)
			}
		}
		return paneKeyResult{handled: true}
	case "n":
		return resolve(optionDeny)
	case "enter":
		if len(options) == 0 {
			return paneKeyResult{handled: true}
		}
		return resolve(options[v.index].Option)
	default:
		return paneKeyResult{handled: true, allowGlobal: true}
	}
}
