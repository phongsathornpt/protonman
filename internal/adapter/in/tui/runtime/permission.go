package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

const permissionViewID = "permission"

type permissionOption int

const (
	optionAllowOnce permissionOption = iota
	optionAllowSession
	optionAllowProject
	optionAllowGlobal
	optionDeny
)

type permissionOptionItem struct {
	option   permissionOption
	label    string
	shortcut string
}

func permissionOptionsFor(request permission.Request, projectTrusted bool, hasWorkDir bool) []permissionOptionItem {
	items := []permissionOptionItem{
		{option: optionAllowOnce, label: "Allow once", shortcut: "y"},
	}
	if permission.SessionGrantEligible(request) {
		items = append(items, permissionOptionItem{
			option:   optionAllowSession,
			label:    "Allow for this request this session",
			shortcut: "s",
		})
	}
	if permission.PersistentRuleEligible(request) {
		if projectTrusted && hasWorkDir {
			items = append(items, permissionOptionItem{
				option:   optionAllowProject,
				label:    "Allow and save to project (.protonman/config.toml)",
				shortcut: "p",
			})
		}
		items = append(items, permissionOptionItem{
			option:   optionAllowGlobal,
			label:    "Allow and save globally (~/.protonman/config.toml)",
			shortcut: "g",
		})
	}
	items = append(items, permissionOptionItem{
		option:   optionDeny,
		label:    "Deny",
		shortcut: "n",
	})
	return items
}

func (v *permissionPaneView) options(m *bubbleModel) []permissionOptionItem {
	projectTrusted := false
	hasWorkDir := false
	if m != nil {
		projectTrusted = m.projectTrusted
		hasWorkDir = m.workDir != ""
	}
	return permissionOptionsFor(v.pending.request, projectTrusted, hasWorkDir)
}

func shortcutHintFor(options []permissionOptionItem) string {
	parts := make([]string, 0, len(options))
	for _, opt := range options {
		switch opt.option {
		case optionAllowOnce:
			parts = append(parts, "y once")
		case optionAllowSession:
			parts = append(parts, "s session")
		case optionAllowProject:
			parts = append(parts, "p project")
		case optionAllowGlobal:
			parts = append(parts, "g global")
		case optionDeny:
			parts = append(parts, "n deny")
		}
	}
	return strings.Join(parts, " · ")
}

type permissionBridge struct {
	requests chan permissionRequest
	done     chan struct{}
	once     sync.Once
}

type permissionRequest struct {
	request  permission.Request
	response chan permissionResponse
}

type permissionResponse struct {
	resolution permission.Resolution
	err        error
}

func newPermissionBridge() *permissionBridge {
	return &permissionBridge{
		requests: make(chan permissionRequest),
		done:     make(chan struct{}),
	}
}

func (b *permissionBridge) Prompt(ctx context.Context, request permission.Request) (permission.Resolution, error) {
	response := make(chan permissionResponse, 1)
	pending := permissionRequest{request: request, response: response}
	select {
	case b.requests <- pending:
	case <-ctx.Done():
		return permission.Resolution{}, fmt.Errorf("permission prompt canceled: %w", ctx.Err())
	case <-b.done:
		return permission.Resolution{}, errors.New("permission prompt closed")
	}
	select {
	case result := <-response:
		return result.resolution, result.err
	case <-ctx.Done():
		return permission.Resolution{}, fmt.Errorf("permission prompt canceled: %w", ctx.Err())
	case <-b.done:
		return permission.Resolution{}, errors.New("permission prompt closed")
	}
}

func (b *permissionBridge) Next() tea.Cmd {
	return func() tea.Msg {
		select {
		case request := <-b.requests:
			return permissionRequestMsg{request: request}
		case <-b.done:
			return permissionBridgeClosedMsg{}
		}
	}
}

func (b *permissionBridge) Close() { b.once.Do(func() { close(b.done) }) }

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
			return true, m.resolvePermission(options[idx].option)
		}
		return true, nil
	case "y":
		return true, m.resolvePermission(optionAllowOnce)
	case "s":
		for _, item := range options {
			if item.option == optionAllowSession {
				return true, m.resolvePermission(optionAllowSession)
			}
		}
		return true, nil
	case "p":
		for _, item := range options {
			if item.option == optionAllowProject {
				return true, m.resolvePermission(optionAllowProject)
			}
		}
		return true, nil
	case "g":
		for _, item := range options {
			if item.option == optionAllowGlobal {
				return true, m.resolvePermission(optionAllowGlobal)
			}
		}
		return true, nil
	case "n":
		return true, m.resolvePermission(optionDeny)
	case "enter":
		return true, m.resolvePermission(options[v.index].option)
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

func (m bubbleModel) permissionCard() string {
	view := m.permissionView()
	if view == nil {
		return ""
	}
	return view.card(&m)
}

func (v *permissionPaneView) card(m *bubbleModel) string {
	request := v.pending.request
	options := v.options(m)
	labels := make([]string, 0, len(options))
	for _, option := range options {
		labels = append(labels, option.label)
	}
	shortcutHint := shortcutHintFor(options)
	title := "Permission required"
	tone := pane.ToneWarning
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
		tone = pane.ToneUser
	case permission.ToolEdit:
		title = "Permission required — modifies workspace"
		tone = pane.ToneError
	case permission.ToolBash:
		var input struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd,omitempty"`
		}
		_ = json.Unmarshal(request.Arguments, &input)
		analysis := tool.AnalyzeCommand(input.Command)
		switch analysis.Scope {
		case tool.CommandScopePublish:
			title, tone = "Permission required — publishes package", pane.ToneError
		case tool.CommandScopeDeployment:
			if analysis.Risk == tool.CommandRiskRemoteDestructive {
				title = "Permission required — destructive deployment change"
			} else {
				title = "Permission required — changes deployment"
			}
			tone = pane.ToneError
		case tool.CommandScopeRemote:
			if analysis.Risk == tool.CommandRiskRemoteDestructive {
				title = "Permission required — destructively modifies remote"
			} else {
				title = "Permission required — modifies remote"
			}
			tone = pane.ToneError
		default:
			switch analysis.Effect {
			case tool.CommandEffectReadOnly:
				title, tone = "Permission request — shell read only", pane.ToneUser
			case tool.CommandEffectMutating:
				title, tone = "Permission required — shell modifies state", pane.ToneError
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
	result := pane.PermissionView(pane.PermissionSnapshot{
		Width:        m.width,
		Height:       m.height,
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
	return renderModalRows(m, paneToneColor(result.Tone), result.Rows)
}

type permissionRequestMsg struct{ request permissionRequest }
type permissionBridgeClosedMsg struct{}

type permissionRuleSavedMsg struct {
	scope string
	rule  permission.Rule
	err   error
}

func (m *bubbleModel) updatePermissionRuleSaved(message permissionRuleSavedMsg) (tea.Model, tea.Cmd) {
	if message.err != nil {
		m.appendError(fmt.Sprintf("Failed to save permission rule to %s: %s", message.scope, message.err))
		m.refreshViewport()
		return m, nil
	}
	m.appendLine(successStyle.Render(fmt.Sprintf("Saved %s rule to %s config (%s: %s).", message.rule.Action, message.scope, message.rule.Tool, message.rule.Pattern)))
	m.refreshViewport()
	if message.scope == "project" {
		if view, _ := m.bottom.find(projectViewID).(*projectPaneView); view != nil {
			view.notice = "Permission rule saved"
			return m, view.reload(m)
		}
	}
	return m, nil
}
