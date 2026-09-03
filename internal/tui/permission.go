package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/permission"
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
			m.viewport.PageUp()
			m.followTail = m.viewport.AtBottom()
			return true, nil
		case "pgdown":
			m.viewport.PageDown()
			m.followTail = m.viewport.AtBottom()
			return true, nil
		case "up", "k":
			m.viewport.ScrollUp(1)
			m.followTail = m.viewport.AtBottom()
			return true, nil
		case "down", "j":
			m.viewport.ScrollDown(1)
			m.followTail = m.viewport.AtBottom()
			return true, nil
		case "y", "s", "n", "ctrl+c", "1", "2", "3", "enter":
			// Decisions remain available while parked.
		default:
			return true, nil
		}
	}

	switch message.String() {
	case "esc":
		v.parked = true
		m.activity = "permission — tab to return"
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
	case "n", "ctrl+c":
		return true, m.resolvePermission(optionDeny)
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
	destructive := request.ToolKind == permission.ToolBash || request.ToolKind == permission.ToolEdit
	title := "Permission required"
	titleStyle := warningStyle
	border := warningColor
	if destructive {
		title = "Permission required — " + string(request.ToolKind)
		titleStyle = errorStyle
		border = accentError
	}
	maxWidth := maxInt(1, m.width-8)
	rows := make([]string, 0, 8)
	rows = append(rows, titleStyle.Render(title))
	rows = append(rows, fmt.Sprintf("%s (%s)", request.ToolName, request.ToolKind))
	rows = append(rows, mutedStyle.Render(wrapWords("Target: "+request.Detail, maxInt(1, maxWidth-6))))
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
		rows = append(rows, mutedStyle.Render("tab return   y/s/n still work   pgup/pgdn scroll"))
	} else {
		rows = append(rows, mutedStyle.Render("j/k move   1-3 select   y once   s session   n deny   esc park"))
	}
	return modalStyle.
		BorderForeground(border).
		MaxWidth(maxInt(1, m.width-4)).
		Render(strings.Join(rows, "\n"))
}

type permissionRequestMsg struct{ request permissionRequest }
type permissionBridgeClosedMsg struct{}
