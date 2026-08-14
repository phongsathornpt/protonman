// Package toolcall orchestrates permission checks and tool execution.
package toolcall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

// PermissionPrompt resolves an interactive permission request.
type PermissionPrompt func(context.Context, permission.Request) (permission.Resolution, error)

// ErrInvalidService indicates that an application service dependency is
// missing or invalid.
var ErrInvalidService = errors.New("invalid tool-call service")

// ErrUnknownTool indicates that a call names no registered tool.
var ErrUnknownTool = errors.New("unknown tool")

// ErrPermissionDenied indicates that a call was stopped before execution.
var ErrPermissionDenied = errors.New("permission denied")

// Option configures a Service during construction.
type Option func(*Service) error

// WithMode sets the initial permission mode.
func WithMode(mode permission.Mode) Option {
	return func(service *Service) error {
		if !validMode(mode) {
			return fmt.Errorf("%w: invalid permission mode %q", ErrInvalidService, mode)
		}
		service.mode = mode
		return nil
	}
}

// WithPrompt sets the interactive resolver used by ask and auto modes.
func WithPrompt(prompt PermissionPrompt) Option {
	return func(service *Service) error {
		service.prompt = prompt
		return nil
	}
}

// Service is the application boundary for every tool call.
type Service struct {
	registry tool.Registry
	policy   *permission.Policy

	mu     sync.RWMutex
	mode   permission.Mode
	prompt PermissionPrompt
	grants map[permission.GrantKey]struct{}
}

// NewService builds a permission-aware tool-call service.
func NewService(registry tool.Registry, policy *permission.Policy, options ...Option) (*Service, error) {
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrInvalidService)
	}
	if policy == nil {
		return nil, fmt.Errorf("%w: policy is required", ErrInvalidService)
	}

	service := &Service{
		registry: registry,
		policy:   policy,
		mode:     permission.ModeAsk,
		grants:   make(map[permission.GrantKey]struct{}),
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(service); err != nil {
			return nil, err
		}
	}
	return service, nil
}

// Mode returns the current permission mode.
func (s *Service) Mode() permission.Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// SetMode changes the permission mode for subsequent calls.
func (s *Service) SetMode(mode permission.Mode) error {
	if !validMode(mode) {
		return fmt.Errorf("invalid permission mode %q", mode)
	}
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
	return nil
}

// Definitions returns the registry snapshot used by the UI or model adapter.
func (s *Service) Definitions() []tool.Definition {
	return s.registry.Definitions()
}

// Call evaluates permission and executes one tool call if authorized.
func (s *Service) Call(ctx context.Context, call tool.Call) (tool.Result, error) {
	if err := call.Validate(); err != nil {
		return tool.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{}, fmt.Errorf("before tool call: %w", err)
	}

	handler, ok := s.registry.Lookup(call.Name)
	if !ok {
		return tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
		}, fmt.Errorf("%w: %s", ErrUnknownTool, call.Name)
	}
	definition := handler.Definition()
	request := permission.Request{
		CallID:    call.ID,
		ToolName:  definition.Name,
		ToolKind:  permission.ToolKind(definition.Kind),
		Detail:    permissionDetail(definition, call.Arguments),
		Arguments: append(json.RawMessage(nil), call.Arguments...),
	}

	resolution := s.authorize(ctx, request)
	if resolution.Action != permission.ActionAllow {
		return tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Denied:   true,
		}, fmt.Errorf("%w: %s", ErrPermissionDenied, resolution.Reason)
	}
	if resolution.Scope == permission.GrantScopeSession {
		s.rememberGrant(request.Key())
	}

	result, err := handler.Execute(ctx, call)
	if result.CallID == "" {
		result.CallID = call.ID
	}
	if result.ToolName == "" {
		result.ToolName = call.Name
	}
	if err != nil {
		return result, fmt.Errorf("execute %s: %w", call.Name, err)
	}
	return result, nil
}

func (s *Service) authorize(ctx context.Context, request permission.Request) permission.Resolution {
	staticDecision := s.policy.Evaluate(request)
	switch staticDecision.Action {
	case permission.ActionDeny:
		return permission.Resolution{
			Action: permission.ActionDeny,
			Reason: staticDecision.Reason,
		}
	case permission.ActionAllow:
		return permission.Resolution{
			Action: permission.ActionAllow,
			Reason: staticDecision.Reason,
		}
	}

	s.mu.RLock()
	_, granted := s.grants[request.Key()]
	mode := s.mode
	prompt := s.prompt
	s.mu.RUnlock()
	if granted {
		return permission.Resolution{
			Action: permission.ActionAllow,
			Reason: "allowed by session grant",
		}
	}

	switch mode {
	case permission.ModeAlwaysApprove:
		return permission.Resolution{
			Action: permission.ActionAllow,
			Reason: "always-approve mode",
		}
	case permission.ModeDeny:
		return permission.Resolution{
			Action: permission.ActionDeny,
			Reason: "deny mode",
		}
	case permission.ModeAsk, permission.ModeAuto:
		if prompt == nil {
			return permission.Resolution{
				Action: permission.ActionDeny,
				Reason: "no permission prompt is configured",
			}
		}
		resolution, err := prompt(ctx, request)
		if err != nil {
			return permission.Resolution{
				Action: permission.ActionDeny,
				Reason: "permission prompt failed: " + err.Error(),
			}
		}
		if resolution.Action != permission.ActionAllow && resolution.Action != permission.ActionDeny {
			return permission.Resolution{
				Action: permission.ActionDeny,
				Reason: "permission prompt returned an invalid decision",
			}
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
		return resolution
	default:
		return permission.Resolution{
			Action: permission.ActionDeny,
			Reason: "invalid permission mode",
		}
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

func validMode(mode permission.Mode) bool {
	switch mode {
	case permission.ModeAsk, permission.ModeAuto, permission.ModeAlwaysApprove, permission.ModeDeny:
		return true
	default:
		return false
	}
}
