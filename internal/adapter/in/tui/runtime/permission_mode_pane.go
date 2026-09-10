package runtime

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

const permissionModeViewID = "permission-mode"

type permissionModeChoice uint8

const (
	permissionModeAsk permissionModeChoice = iota
	permissionModePlan
	permissionModeAlwaysApprove
)

type permissionModePaneView struct{ index int }

func (*permissionModePaneView) ID() string                             { return permissionModeViewID }
func (*permissionModePaneView) PresentationMode() panePresentationMode { return paneBelowComposer }

func (v *permissionModePaneView) Render(ctx paneRenderContext) string {
	labels := []string{"Ask", "Plan", "Always Approve"}
	rows := make([]string, 0, len(labels))
	for i, label := range labels {
		marker := "  "
		style := mutedStyle
		if i == v.index {
			marker = "> "
			style = userStyle
		}
		rows = append(rows, marker+style.Render(label))
	}
	help := paneKeyboardHelp(ctx.width-4, "↑/↓", "Navigate", "enter", "Select", "esc/q", "Go Back")
	return renderModalRows(ctx, accentAssistant, paneSection("Permission Mode", rows, help, "", ctx.width))
}

func (v *permissionModePaneView) HandlePaneKey(_ paneRenderContext, message tea.KeyPressMsg) paneKeyResult {
	switch {
	case key.Matches(message, paneKeys.Close):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionClose, paneID: permissionModeViewID}}
	case key.Matches(message, paneKeys.Up):
		if v.index > 0 {
			v.index--
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneKeys.Down):
		if v.index < 2 {
			v.index++
		}
		return paneKeyResult{handled: true}
	case key.Matches(message, paneKeys.Confirm):
		return paneKeyResult{handled: true, action: paneAction{kind: paneActionSetPermissionMode, permissionMode: permissionModeChoice(v.index)}}
	default:
		return paneKeyResult{handled: true}
	}
}

func currentPermissionModeChoice(m *bubbleModel) permissionModeChoice {
	if m != nil && m.planMode {
		return permissionModePlan
	}
	if m != nil && m.service != nil && m.service.Mode() == permission.ModeAlwaysApprove {
		return permissionModeAlwaysApprove
	}
	return permissionModeAsk
}

func (m *bubbleModel) syncPermissionModePane() {
	if view, _ := m.panes.bottom.find(permissionModeViewID).(*permissionModePaneView); view != nil {
		view.index = int(currentPermissionModeChoice(m))
	}
}

func (m *bubbleModel) openPermissionModePane() {
	if m.panes.bottom.has(permissionModeViewID) {
		m.panes.bottom.remove(permissionModeViewID)
		m.requestRelayout()
		return
	}
	m.panes.bottom.push(&permissionModePaneView{index: int(currentPermissionModeChoice(m))})
	m.requestRelayout()
}

func (m *bubbleModel) applyPermissionModeChoice(choice permissionModeChoice) {
	m.setPlanEnabled(false)
	switch choice {
	case permissionModePlan:
		_ = m.setPermissionMode(permission.ModeAsk)
		m.setPlanEnabled(true)
	case permissionModeAlwaysApprove:
		_ = m.setPermissionMode(permission.ModeAlwaysApprove)
	default:
		_ = m.setPermissionMode(permission.ModeAsk)
	}
	m.syncPermissionModePane()
	m.requestRelayout()
}

func (m *bubbleModel) permissionModeLabel() string {
	if m.planMode {
		return "plan"
	}
	if m.service == nil {
		return "ask"
	}
	mode := strings.TrimSpace(m.service.Mode().String())
	if mode == "always-approve" {
		return "auto"
	}
	if mode == "deny" {
		return "deny"
	}
	return "ask"
}
