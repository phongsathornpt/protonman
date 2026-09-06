package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
)

const permissionViewID = "permission"

type permissionOption int

const (
	optionAllowOnce permissionOption = iota
	optionAllowSession
	optionDeny
)

var permissionOptions = []struct {
	option permissionOption
	label  string
}{
	{optionAllowOnce, "Allow once"},
	{optionAllowSession, "Allow for this request this session"},
	{optionDeny, "Deny"},
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

func (v *permissionPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
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
		case "y", "s", "n", "1", "2", "3", "enter":
			// Decisions remain available while reviewing the transcript.
		case "ctrl+c":
			return false, nil
		default:
			return true, nil
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
		if v.index < len(permissionOptions)-1 {
			v.index++
		}
		return true, nil
	case "1":
		return true, m.resolvePermission(optionAllowOnce)
	case "2":
		return true, m.resolvePermission(optionAllowSession)
	case "3":
		return true, m.resolvePermission(optionDeny)
	case "y":
		return true, m.resolvePermission(optionAllowOnce)
	case "s":
		return true, m.resolvePermission(optionAllowSession)
	case "n":
		return true, m.resolvePermission(optionDeny)
	case "ctrl+c":
		return false, nil
	case "enter":
		return true, m.resolvePermission(permissionOptions[v.index].option)
	default:
		return true, nil
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

func (m *bubbleModel) updatePermission(message tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	return nil
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
	if v.parked {
		line := fmt.Sprintf("! Permission pending · %s · tab review · y once · s session · n deny", request.ToolName)
		return mutedStyle.Render(truncateWithEllipsis(line, maxInt(1, m.width-2)))
	}
	title := "Permission required"
	titleStyle := warningStyle
	border := warningColor
	detailExtras := make([]string, 0, 2)
	switch request.ToolKind {
	case permission.ToolRead, permission.ToolGrep, permission.ToolTask, permission.ToolAgent:
		if request.ToolKind == permission.ToolTask {
			title = "Task state update"
		} else if request.ToolKind == permission.ToolAgent {
			title = "Agent orchestration"
		} else {
			title = "Permission request — read only"
		}
		titleStyle = userStyle
		border = accentUser
	case permission.ToolEdit:
		title = "Permission required — modifies workspace"
		titleStyle = errorStyle
		border = accentError
	case permission.ToolBash:
		var input struct {
			Command string `json:"command"`
			Cwd     string `json:"cwd,omitempty"`
		}
		_ = json.Unmarshal(request.Arguments, &input)
		analysis := tool.AnalyzeCommand(input.Command)
		switch analysis.Scope {
		case tool.CommandScopePublish:
			title = "Permission required — publishes package"
			titleStyle = errorStyle
			border = accentError
		case tool.CommandScopeDeployment:
			if analysis.Risk == tool.CommandRiskRemoteDestructive {
				title = "Permission required — destructive deployment change"
			} else {
				title = "Permission required — changes deployment"
			}
			titleStyle = errorStyle
			border = accentError
		case tool.CommandScopeRemote:
			if analysis.Risk == tool.CommandRiskRemoteDestructive {
				title = "Permission required — destructively modifies remote"
			} else {
				title = "Permission required — modifies remote"
			}
			titleStyle = errorStyle
			border = accentError
		default:
			switch analysis.Effect {
			case tool.CommandEffectReadOnly:
				title = "Permission request — shell read only"
				titleStyle = userStyle
				border = accentUser
			case tool.CommandEffectMutating:
				title = "Permission required — shell modifies state"
				titleStyle = errorStyle
				border = accentError
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
	if layoutModeForHeight(m.height) == layoutTiny {
		contentWidth := maxInt(8, m.width-8)
		selected := permissionOptions[v.index].label
		rows := []string{
			titleStyle.Render(truncateWithEllipsis(title, contentWidth)),
			mutedStyle.Render(truncateWithEllipsis(request.ToolName+" · "+request.Detail, contentWidth)),
			brandStyle.Render(glyphPrompt + selected),
			mutedStyle.Render("y once · s session · n deny"),
			mutedStyle.Render("esc review"),
		}
		return renderModalRows(m, border, rows)
	}
	maxWidth := maxInt(1, m.width-8)
	rows := make([]string, 0, 8)
	rows = append(rows, titleStyle.Render(title))
	rows = append(rows, fmt.Sprintf("%s (%s)", request.ToolName, request.ToolKind))
	detailLines := wrapLines("Target: "+request.Detail, maxInt(1, maxWidth-6))
	for _, extra := range detailExtras {
		detailLines = append(detailLines, wrapLines(extra, maxInt(1, maxWidth-6))...)
	}
	maxDetailLines := 6
	if layoutModeForHeight(m.height) == layoutCompact {
		maxDetailLines = 2
	}
	if len(detailLines) > maxDetailLines {
		omitted := len(detailLines) - maxDetailLines
		detailLines = append(detailLines[:maxDetailLines], fmt.Sprintf("... (%d more lines truncated)", omitted))
	}
	rows = append(rows, mutedStyle.Render(strings.Join(detailLines, "\n")))
	rows = append(rows, "")
	for i, option := range permissionOptions {
		marker := "  "
		if i == v.index && !v.parked {
			marker = glyphPrompt
			rows = append(rows, brandStyle.Render(marker+option.label))
			continue
		}
		rows = append(rows, mutedStyle.Render(marker+option.label))
	}
	rows = append(rows, "")
	if v.parked {
		rows = append(rows, mutedStyle.Render("tab review approval   y/s/n decide   pgup/pgdn scroll"))
	} else {
		rows = append(rows, mutedStyle.Render("j/k move   1-3 select   y once   s session   n deny   esc review transcript"))
	}
	if layoutModeForHeight(m.height) == layoutCompact {
		rows = compactPickerRows(rows)
	}
	return renderModalRows(m, border, rows)
}

type permissionRequestMsg struct{ request permissionRequest }
type permissionBridgeClosedMsg struct{}
