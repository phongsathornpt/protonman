package runtime

import (
	"charm.land/bubbles/v2/key"
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

func (*permissionPaneView) ID() string                             { return permissionViewID }
func (*permissionPaneView) PresentationMode() panePresentationMode { return paneBlocking }
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
		switch {
		case key.Matches(message, paneKeys.Tab):
			v.parked = false
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionPermissionActivity, activity: "waiting for permission"}}
		case key.Matches(message, paneKeys.Page):
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionScrollPage, key: message}}
		case key.Matches(message, paneKeys.Up):
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionScrollLines, scrollLines: -1}}
		case key.Matches(message, paneKeys.Down):
			return paneKeyResult{handled: true, action: paneAction{kind: paneActionScrollLines, scrollLines: 1}}
		case message.Text == "y" || message.Text == "s" || message.Text == "p" || message.Text == "g" || message.Text == "n" ||
			(message.Text >= "1" && message.Text <= "5") || key.Matches(message, paneKeys.Confirm):
			// Decisions remain available while reviewing the transcript.
		default:
			return paneKeyResult{handled: true}
		}
	}

	resolve := func(option permissionOption) paneKeyResult {
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionPermissionResolve, permission: option}}
	}
	switch {
	case key.Matches(message, paneKeys.Escape):
		v.parked = true
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionPermissionActivity, activity: "permission pending — tab to review"}}
	case key.Matches(message, paneKeys.Up):
		if v.index > 0 {
			v.index--
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneKeys.Down):
		if v.index < len(options)-1 {
			v.index++
		}
		return paneKeyResult{handled: true}
	case message.Text >= "1" && message.Text <= "5":
		idx := int(message.Text[0] - '1')
		if idx >= 0 && idx < len(options) {
			return resolve(options[idx].Option)
		}
		return paneKeyResult{handled: true}
	case message.Text == "y":
		return resolve(optionAllowOnce)
	case message.Text == "s":
		for _, item := range options {
			if item.Option == optionAllowSession {
				return resolve(optionAllowSession)
			}
		}
		return paneKeyResult{handled: true}
	case message.Text == "p":
		for _, item := range options {
			if item.Option == optionAllowProject {
				return resolve(optionAllowProject)
			}
		}
		return paneKeyResult{handled: true}
	case message.Text == "g":
		for _, item := range options {
			if item.Option == optionAllowGlobal {
				return resolve(optionAllowGlobal)
			}
		}
		return paneKeyResult{handled: true}
	case message.Text == "n":
		return resolve(optionDeny)
	case key.Matches(message, paneKeys.Confirm):
		if len(options) == 0 {
			return paneKeyResult{handled: true}
		}
		return resolve(options[v.index].Option)
	default:
		return paneKeyResult{handled: true}
	}
}
