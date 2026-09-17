package acp

import (
	"context"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

const methodRequestPermission = "session/request_permission"

const (
	permissionOptionAllowOnce   = "allow_once"
	permissionOptionAllowSession = "allow_session"
	permissionOptionRejectOnce  = "reject_once"
)

func (s *Server) requestPermission(ctx context.Context, sessionID string, request permission.Request) (permission.Resolution, error) {
	options := []PermissionOption{
		{OptionID: permissionOptionAllowOnce, Name: "Allow once", Kind: PermissionOptionAllowOnce},
		{OptionID: permissionOptionRejectOnce, Name: "Reject", Kind: PermissionOptionRejectOnce},
	}
	if permission.SessionGrantEligible(request) {
		options = append(options, PermissionOption{
			OptionID: permissionOptionAllowSession,
			Name:     "Allow for this session",
			Kind:     PermissionOptionAllowAlways,
		})
	}
	params := RequestPermissionParams{
		SessionID: sessionID,
		ToolCall:  permissionToolCall(request),
		Options:   options,
	}
	var result RequestPermissionResult
	if err := s.requestClient(ctx, methodRequestPermission, params, &result); err != nil {
		return permission.Resolution{Action: permission.ActionDeny, Scope: permission.GrantScopeOnce, Reason: "permission client request failed"}, err
	}

	switch strings.TrimSpace(result.Outcome.Outcome) {
	case "cancelled":
		return permission.Resolution{Action: permission.ActionDeny, Scope: permission.GrantScopeOnce, Reason: "permission request cancelled"}, nil
	case "selected":
		switch result.Outcome.OptionID {
		case permissionOptionAllowOnce:
			return permission.Resolution{Action: permission.ActionAllow, Scope: permission.GrantScopeOnce, Reason: "approved by ACP client"}, nil
		case permissionOptionAllowSession:
			if !permission.SessionGrantEligible(request) {
				return permission.Resolution{Action: permission.ActionDeny, Scope: permission.GrantScopeOnce, Reason: "session grant is not eligible for this request"}, nil
			}
			return permission.Resolution{Action: permission.ActionAllow, Scope: permission.GrantScopeSession, Reason: "approved for session by ACP client"}, nil
		case permissionOptionRejectOnce:
			return permission.Resolution{Action: permission.ActionDeny, Scope: permission.GrantScopeOnce, Reason: "rejected by ACP client"}, nil
		default:
			return permission.Resolution{Action: permission.ActionDeny, Scope: permission.GrantScopeOnce, Reason: "ACP client selected an unknown permission option"}, fmt.Errorf("unknown permission option %q", result.Outcome.OptionID)
		}
	default:
		return permission.Resolution{Action: permission.ActionDeny, Scope: permission.GrantScopeOnce, Reason: "ACP client returned an invalid permission outcome"}, fmt.Errorf("invalid permission outcome %q", result.Outcome.Outcome)
	}
}

func permissionToolCall(request permission.Request) map[string]any {
	title := strings.TrimSpace(request.ToolName)
	if detail := strings.TrimSpace(request.Detail); detail != "" {
		if title != "" {
			title += " · " + detail
		} else {
			title = detail
		}
	}
	if title == "" {
		title = "Tool permission"
	}
	call := map[string]any{
		"toolCallId": request.CallID,
		"title":      title,
		"kind":       permissionACPToolKind(request.ToolKind),
		"status":     string(ToolCallStatusPending),
	}
	if len(request.Arguments) > 0 {
		call["rawInput"] = string(request.Arguments)
	}
	return call
}

func permissionACPToolKind(kind permission.ToolKind) string {
	switch kind {
	case permission.ToolRead:
		return string(ToolKindRead)
	case permission.ToolEdit:
		return string(ToolKindEdit)
	case permission.ToolGrep, permission.ToolGit:
		return string(ToolKindSearch)
	case permission.ToolBash:
		return string(ToolKindExecute)
	case permission.ToolWeb:
		return string(ToolKindFetch)
	case permission.ToolAgent, permission.ToolTask, permission.ToolCompute:
		return string(ToolKindThink)
	case permission.ToolMCP:
		return string(ToolKindOther)
	default:
		_ = tool.Kind(kind)
		return string(ToolKindOther)
	}
}
