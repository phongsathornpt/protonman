package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/projectTHORN/proton/internal/domain/permission"
)

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

func (b *permissionBridge) Prompt(
	ctx context.Context,
	request permission.Request,
) (permission.Resolution, error) {
	response := make(chan permissionResponse, 1)
	pending := permissionRequest{
		request:  request,
		response: response,
	}
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

func (b *permissionBridge) Close() {
	b.once.Do(func() { close(b.done) })
}

func (m *bubbleModel) updatePermission(message tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modalParked {
		switch message.String() {
		case "tab":
			m.modalParked = false
			m.activity = "waiting for permission"
			return m, nil
		case "pgup":
			m.viewport.PageUp()
			m.followTail = m.viewport.AtBottom()
			return m, nil
		case "pgdown":
			m.viewport.PageDown()
			m.followTail = m.viewport.AtBottom()
			return m, nil
		case "up", "k":
			m.viewport.LineUp(1)
			m.followTail = m.viewport.AtBottom()
			return m, nil
		case "down", "j":
			m.viewport.LineDown(1)
			m.followTail = m.viewport.AtBottom()
			return m, nil
		case "y", "s", "n", "ctrl+c", "1", "2", "3", "enter":
			// Resolve even while parked so a decision is never blocked.
		default:
			return m, nil
		}
	}

	switch message.String() {
	case "esc":
		m.modalParked = true
		m.activity = "permission — tab to return"
		return m, nil
	case "up", "k":
		if m.permIndex > 0 {
			m.permIndex--
		}
		return m, nil
	case "down", "j":
		if m.permIndex < len(permissionOptions)-1 {
			m.permIndex++
		}
		return m, nil
	case "1":
		return m.resolvePermission(optionAllowOnce)
	case "2":
		return m.resolvePermission(optionAllowSession)
	case "3":
		return m.resolvePermission(optionDeny)
	case "y":
		return m.resolvePermission(optionAllowOnce)
	case "s":
		return m.resolvePermission(optionAllowSession)
	case "n", "ctrl+c":
		return m.resolvePermission(optionDeny)
	case "enter":
		return m.resolvePermission(permissionOptions[m.permIndex].option)
	default:
		return m, nil
	}
}

func (m *bubbleModel) resolvePermission(option permissionOption) (tea.Model, tea.Cmd) {
	if m.modal == nil {
		return m, nil
	}
	var resolution permission.Resolution
	switch option {
	case optionAllowOnce:
		resolution = permission.Resolution{
			Action: permission.ActionAllow,
			Reason: "user allowed one call",
		}
	case optionAllowSession:
		resolution = permission.Resolution{
			Action: permission.ActionAllow,
			Scope:  permission.GrantScopeSession,
			Reason: "user allowed this exact request for the session",
		}
	default:
		resolution = permission.Resolution{
			Action: permission.ActionDeny,
			Reason: "user denied one call",
		}
	}
	m.modal.response <- permissionResponse{resolution: resolution}
	m.modal = nil
	m.modalParked = false
	m.permIndex = 0
	m.activity = m.pendingActivity
	if m.activity == "" || m.activity == "waiting for permission" {
		m.activity = "running tool"
	}
	return m, nil
}

func (m bubbleModel) permissionCard() string {
	request := m.modal.request
	destructive := request.ToolKind == permission.ToolBash || request.ToolKind == permission.ToolEdit
	title := "Permission required"
	titleStyle := warningStyle
	border := warningColor
	if destructive {
		title = "Permission required — " + string(request.ToolKind)
		titleStyle = errorStyle
		border = accentError
	}
	maxWidth := maxInt(32, m.width-8)
	rows := make([]string, 0, 8)
	rows = append(rows, titleStyle.Render(title))
	rows = append(rows, fmt.Sprintf("%s (%s)", request.ToolName, request.ToolKind))
	rows = append(rows, mutedStyle.Render(wrapWords("Target: "+request.Detail, maxWidth-6)))
	rows = append(rows, "")
	for i, option := range permissionOptions {
		marker := "  "
		label := option.label
		if i == m.permIndex && !m.modalParked {
			marker = glyphPrompt
			rows = append(rows, brandStyle.Render(marker+label))
			continue
		}
		rows = append(rows, mutedStyle.Render(marker+label))
	}
	rows = append(rows, "")
	if m.modalParked {
		rows = append(rows, mutedStyle.Render("tab return   y/s/n still work   pgup/pgdn scroll"))
	} else {
		rows = append(rows, mutedStyle.Render("j/k move   1-3 select   y once   s session   n deny   esc read"))
	}
	return modalStyle.
		BorderForeground(border).
		MaxWidth(maxInt(24, m.width-4)).
		Render(strings.Join(rows, "\n"))
}

type permissionRequestMsg struct {
	request permissionRequest
}

type permissionBridgeClosedMsg struct{}
