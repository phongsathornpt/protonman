package runtime

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/permissionpolicy"
	panecommon "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/common"
	permissionpane "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane/permission"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
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

func (v *permissionPaneView) card(ctx paneRenderContext) string {
	request := v.pending.request
	options := permissionpolicy.Options(v.pending.request, ctx.projectTrusted, ctx.hasWorkDir)
	labels := make([]string, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.Label)
	}
	shortcutHint := shortcutHintFor(options)
	title := "Permission required"
	tone := panecommon.ToneWarning
	detailExtras := make([]string, 0, 3)
	switch request.ToolKind {
	case permission.ToolRead, permission.ToolGrep, permission.ToolTask, permission.ToolAgent:
		switch request.ToolKind {
		case permission.ToolTask:
			title = "Task plan change"
		case permission.ToolAgent:
			title = "Agent orchestration"
		default:
			title = "Permission request — read only"
		}
		tone = panecommon.ToneUser
	case permission.ToolEdit:
		title = "Permission required — modifies workspace"
		tone = panecommon.ToneError
	case permission.ToolBash:
		var input struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd,omitempty"`
		}
		_ = json.Unmarshal(request.Arguments, &input)
		analysis := tool.AnalyzeCommand(input.Command)
		switch analysis.Scope {
		case tool.CommandScopePublish:
			title, tone = "Permission required — publishes package", panecommon.ToneError
		case tool.CommandScopeDeployment:
			if analysis.Risk == tool.CommandRiskRemoteDestructive {
				title = "Permission required — destructive deployment change"
			} else {
				title = "Permission required — changes deployment"
			}
			tone = panecommon.ToneError
		case tool.CommandScopeRemote:
			if analysis.Risk == tool.CommandRiskRemoteDestructive {
				title = "Permission required — destructively modifies remote"
			} else {
				title = "Permission required — modifies remote"
			}
			tone = panecommon.ToneError
		default:
			switch analysis.Effect {
			case tool.CommandEffectReadOnly:
				title, tone = "Permission request — shell read only", panecommon.ToneUser
			case tool.CommandEffectMutating:
				title, tone = "Permission required — shell modifies state", panecommon.ToneError
			default:
				title = "Permission required — shell effects unknown"
			}
		}
		cwd := strings.TrimSpace(input.Cwd)
		if cwd == "" {
			cwd = "."
		}
		detailExtras = append(detailExtras, "Cwd: "+cwd)
		if analysis.Scope != tool.CommandScopeUnknown {
			detailExtras = append(detailExtras, "Scope: "+string(analysis.Scope))
		}
		if analysis.Reason != "" {
			detailExtras = append(detailExtras, fmt.Sprintf("Effect: %s · %s", analysis.Effect, analysis.Reason))
		}
	}
	result := permissionpane.PermissionView(permissionpane.PermissionSnapshot{
		Width:        ctx.width,
		Height:       ctx.height,
		Parked:       v.parked,
		Index:        v.index,
		Title:        title,
		Tone:         tone,
		ToolName:     tool.DisplayName(request.ToolName),
		ToolKind:     string(request.ToolKind),
		Detail:       request.Detail,
		DetailExtras: detailExtras,
		Options:      labels,
		ShortcutHint: shortcutHint,
	})
	if result.Inline != "" {
		return result.Inline
	}
	rows := result.Rows
	if len(rows) > 1 && layoutModeForHeight(ctx.height) == layoutNormal {
		rows = appendPaneGroup(rows[:1], rows[1:]...)
	}
	helpBindings := []string{"↑/↓", "Navigate", "enter", "Choose", "esc", "Review"}
	if v.parked {
		helpBindings = []string{"tab", "Review", "pgup/pgdn", "Scroll", "esc", "Back"}
	}
	rows = appendPaneGroup(rows, paneKeyboardHelp(maxInt(1, ctx.width-6), helpBindings...))
	status := tool.DisplayName(request.ToolName)
	if len(labels) > 0 {
		index := maxInt(0, minInt(v.index, len(labels)-1))
		status += " · " + labels[index]
	}
	rows = append(rows, paneRightStatus(maxInt(1, ctx.width-6), status))
	return renderModalRows(ctx, paneToneColor(result.Tone), rows)
}

type permissionRequestMsg struct{ request permissionRequest }
type permissionBridgeClosedMsg struct{}

type permissionRuleSavedMsg struct {
	scope string
	rule  permission.Rule
	err   error
}

func (m *bubbleModel) updatePermissionRuleSaved(message permissionRuleSavedMsg) tea.Cmd {
	if message.err != nil {
		m.appendError(fmt.Sprintf("Failed to save permission rule to %s: %s", message.scope, message.err))
		m.refreshViewport()
		return nil
	}
	m.appendLine(successStyle.Render(fmt.Sprintf("Saved %s rule to %s config (%s: %s).", message.rule.Action, message.scope, message.rule.Tool, message.rule.Pattern)))
	m.refreshViewport()
	return nil
}

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
	m.requestRelayout()
	m.reconcileLayout()
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
				err := m.application.Projects.SavePermissionRule(workDir, rule)
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
				err := m.application.UserSettings.SavePermissionRule(rule)
				return permissionRuleSavedMsg{scope: "global", rule: rule, err: err}
			}
		}
	default:
		resolution = permission.Resolution{Action: permission.ActionDeny, Reason: "user denied one call"}
	}
	view.pending.response <- permissionResponse{resolution: resolution}
	m.panes.bottom.remove(permissionViewID)
	m.requestRelayout()
	m.reconcileLayout()
	m.activity = m.pendingActivity
	if m.activity == "" || m.activity == "waiting for permission" {
		m.activity = "running tool"
	}
	m.syncSlashView()
	return saveCmd
}
