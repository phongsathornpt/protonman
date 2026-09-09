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
		case "pgup", "pgdown":
			return true, m.updateConversationViewport(message)
		case "up", "k":
			m.scrollConversationLines(-1)
			return true, nil
		case "down", "j":
			m.scrollConversationLines(1)
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
