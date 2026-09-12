package toolcall

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func (s *Service) guardCall(ctx context.Context, request permission.Request) error {
	s.mu.RLock()
	guard := s.guard
	s.mu.RUnlock()
	if guard == nil {
		return nil
	}
	return guard(ctx, request)
}

func (s *Service) authorize(ctx context.Context, request permission.Request) (permission.Resolution, error) {
	staticDecision := s.policy.Evaluate(request)
	switch staticDecision.Action {
	case permission.ActionDeny:
		return permission.Resolution{
			Action: permission.ActionDeny,
			Reason: staticDecision.Reason,
		}, nil
	case permission.ActionAllow:
		return permission.Resolution{
			Action: permission.ActionAllow,
			Reason: staticDecision.Reason,
		}, nil
	}

	s.mu.RLock()
	_, granted := s.grants[request.Key()]
	mode := s.mode
	prompt := s.prompt
	s.mu.RUnlock()
	if mode == permission.ModeDeny {
		return permission.Resolution{
			Action: permission.ActionDeny,
			Reason: "deny mode",
		}, nil
	}
	if request.ToolKind == permission.ToolAgent && (mode == permission.ModeAsk || mode == permission.ModeAuto) {
		return permission.Resolution{Action: permission.ActionAllow, Reason: "subagent orchestration is allowed"}, nil
	}
	if taskMetadataAutoAllowed(request) && (mode == permission.ModeAsk || mode == permission.ModeAuto) {
		return permission.Resolution{Action: permission.ActionAllow, Reason: "safe task metadata transition is allowed"}, nil
	}
	if granted && request.Effect == tool.CommandEffectReadOnly && request.Risk == tool.CommandRiskNormal {
		return permission.Resolution{
			Action: permission.ActionAllow,
			Reason: "allowed by session grant",
		}, nil
	}

	switch mode {
	case permission.ModeAlwaysApprove:
		return permission.Resolution{
			Action: permission.ActionAllow,
			Reason: "always-approve mode",
		}, nil
	case permission.ModeAsk, permission.ModeAuto:
		if prompt == nil {
			return permission.Resolution{
				Action: permission.ActionDeny,
				Reason: "no permission prompt is configured",
			}, nil
		}
		resolution, err := prompt(ctx, request)
		if err != nil {
			return permission.Resolution{
				Action: permission.ActionDeny,
				Reason: "permission prompt failed: " + err.Error(),
			}, fmt.Errorf("permission prompt: %w", err)
		}
		if resolution.Action != permission.ActionAllow && resolution.Action != permission.ActionDeny {
			return permission.Resolution{
				Action: permission.ActionDeny,
				Reason: "permission prompt returned an invalid decision",
			}, nil
		}
		if resolution.Reason == "" {
			resolution.Reason = "interactive permission decision"
		}
		if resolution.Action == permission.ActionDeny {
			resolution.Scope = permission.GrantScopeOnce
		}
		if resolution.Scope != permission.GrantScopeOnce && resolution.Scope != permission.GrantScopeSession {
			resolution.Scope = permission.GrantScopeOnce
		}
		if resolution.Scope == permission.GrantScopeSession && !permission.SessionGrantEligible(request) {
			resolution.Scope = permission.GrantScopeOnce
		}
		return resolution, nil
	default:
		return permission.Resolution{
			Action: permission.ActionDeny,
			Reason: "invalid permission mode",
		}, nil
	}
}

func (s *Service) rememberGrant(key permission.GrantKey) {
	s.mu.Lock()
	s.grants[key] = struct{}{}
	s.mu.Unlock()
}

func permissionDetail(definition tool.Definition, arguments json.RawMessage) string {
	if definition.PermissionDetailKey == "" {
		return string(arguments)
	}

	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(arguments, &fields); err != nil {
		return string(arguments)
	}
	field, ok := fields[definition.PermissionDetailKey]
	if !ok {
		return string(arguments)
	}
	var value string
	if err := json.Unmarshal(field, &value); err == nil {
		return value
	}
	return string(field)
}

func (s *Service) executionContext(parent context.Context, definition tool.Definition) (context.Context, context.CancelFunc) {
	if definition.ExecutionTimeoutPolicy == tool.ExecutionTimeoutCallerBounded {
		return context.WithCancel(parent)
	}
	s.mu.RLock()
	timeout := s.executionTimeout
	s.mu.RUnlock()
	return boundedContext(parent, timeout)
}

func (s *Service) permissionContext(parent context.Context) (context.Context, context.CancelFunc) {
	s.mu.RLock()
	timeout := s.permissionTimeout
	s.mu.RUnlock()
	return boundedContext(parent, timeout)
}

func boundedContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}
