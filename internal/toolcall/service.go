// Package toolcall orchestrates permission checks and tool execution.
package toolcall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"strings"
	"sync"
	"time"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// PermissionPrompt resolves an interactive permission request.
type PermissionPrompt func(context.Context, permission.Request) (permission.Resolution, error)

// CallGuard can fail closed before static policy, grants, or permission mode
// evaluation. Adapters use it for temporary execution constraints such as a
// read-only planning mode without weakening the shared policy layer.
type CallGuard func(context.Context, permission.Request) error

// ErrInvalidService indicates that an application service dependency is
// missing or invalid.
var ErrInvalidService = errors.New("invalid tool-call service")

// ErrUnknownTool indicates that a call names no registered tool.
var ErrUnknownTool = errors.New("unknown tool")

// ErrPermissionDenied indicates that a call was stopped before execution.
var ErrPermissionDenied = errors.New("permission denied")

// Option configures a Service during construction.
type Option func(*Service) error

const (
	// DefaultPermissionTimeout bounds policy evaluation and interactive
	// permission resolution when callers do not provide a stricter timeout.
	DefaultPermissionTimeout = runtimepolicy.ToolPermissionTimeout
	// DefaultExecutionTimeout bounds one permission-approved tool call when the
	// caller does not provide a stricter context.
	DefaultExecutionTimeout = runtimepolicy.ToolExecutionTimeout
)

// WithMode sets the initial permission mode.
func WithMode(mode permission.Mode) Option {
	return func(service *Service) error {
		if !mode.Valid() {
			return fmt.Errorf("%w: invalid permission mode %q", ErrInvalidService, mode)
		}
		service.mode = mode
		return nil
	}
}

// WithObserver attaches a redacted lifecycle event observer.
func WithObserver(observer Observer) Option {
	return func(service *Service) error {
		if observer == nil {
			return fmt.Errorf("%w: observer is required", ErrInvalidService)
		}
		service.observer = observer
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

// WithPermissionTimeout bounds policy evaluation and interactive permission
// resolution. Zero disables this service-level bound.
func WithPermissionTimeout(timeout time.Duration) Option {
	return func(service *Service) error {
		if timeout < 0 {
			return fmt.Errorf("%w: permission timeout cannot be negative", ErrInvalidService)
		}
		service.permissionTimeout = timeout
		return nil
	}
}

// WithExecutionTimeout bounds approved handler execution.
// Zero disables the service-level bound; callers should normally keep the
// non-zero default for process-backed tools.
func WithExecutionTimeout(timeout time.Duration) Option {
	return func(service *Service) error {
		if timeout < 0 {
			return fmt.Errorf("%w: execution timeout cannot be negative", ErrInvalidService)
		}
		service.executionTimeout = timeout
		return nil
	}
}

// Service is the application boundary for every tool call.
type Service struct {
	registry tool.Registry
	policy   *permission.Policy
	observer Observer

	mu     sync.RWMutex
	mode   permission.Mode
	prompt PermissionPrompt
	guard  CallGuard
	grants map[permission.GrantKey]struct{}

	permissionTimeout time.Duration
	executionTimeout  time.Duration
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
		registry:          registry,
		policy:            policy,
		mode:              permission.ModeAsk,
		grants:            make(map[permission.GrantKey]struct{}),
		permissionTimeout: DefaultPermissionTimeout,
		executionTimeout:  DefaultExecutionTimeout,
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

// Clone creates an independent permission state over the same immutable
// registry and policy. Session grants are intentionally not copied.
func (s *Service) Clone() *Service {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return &Service{
		registry:          s.registry,
		policy:            s.policy,
		observer:          s.observer,
		mode:              s.mode,
		prompt:            s.prompt,
		guard:             s.guard,
		grants:            make(map[permission.GrantKey]struct{}),
		permissionTimeout: s.permissionTimeout,
		executionTimeout:  s.executionTimeout,
	}
}

// Mode returns the current permission mode.
func (s *Service) Mode() permission.Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mode
}

// SetMode changes the permission mode for subsequent calls.
func (s *Service) SetMode(mode permission.Mode) error {
	if !mode.Valid() {
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

// SetPrompt replaces the interactive resolver used by ask and auto modes.
func (s *Service) SetPrompt(prompt PermissionPrompt) {
	s.mu.Lock()
	s.prompt = prompt
	s.mu.Unlock()
}

// SetCallGuard replaces the temporary fail-closed guard evaluated before the
// normal permission policy. Passing nil removes the guard.
func (s *Service) SetCallGuard(guard CallGuard) {
	s.mu.Lock()
	s.guard = guard
	s.mu.Unlock()
}

// Call evaluates permission and executes one tool call if authorized.
func (s *Service) Call(ctx context.Context, call tool.Call) (tool.Result, error) {
	telemetry := callTelemetry{
		started: time.Now(),
		call:    call,
	}
	s.observeCallStarted(ctx, telemetry)
	if err := call.Validate(); err != nil {
		result := tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Failure:  tool.FailureFromError(err),
		}
		s.observeCallResult(ctx, telemetry, result, err)
		return result, err
	}
	if err := ctx.Err(); err != nil {
		wrappedErr := fmt.Errorf("before tool call: %w", err)
		result := tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Failure:  tool.FailureFromError(wrappedErr),
		}
		s.observeCallResult(ctx, telemetry, result, wrappedErr)
		return result, wrappedErr
	}
	handler, ok := s.registry.Lookup(call.Name)
	if !ok {
		unknownErr := fmt.Errorf("%w: %s", ErrUnknownTool, call.Name)
		result := tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Failure:  &tool.Failure{Code: tool.ErrorCodeUnknownTool, Message: unknownErr.Error()},
		}
		s.observeCallResult(ctx, telemetry, result, unknownErr)
		return result, unknownErr
	}
	definition := handler.Definition()
	telemetry.toolKind = definition.Kind
	detail := permissionDetail(definition, call.Arguments)
	if provider, ok := handler.(tool.DetailProvider); ok {
		if custom := strings.TrimSpace(provider.PermissionDetail(call.Arguments)); custom != "" {
			detail = custom
		}
	}
	request := permission.Request{
		CallID:    call.ID,
		ToolName:  definition.Name,
		ToolKind:  definition.Kind,
		Detail:    detail,
		Arguments: append(json.RawMessage(nil), call.Arguments...),
		Risk:      tool.EffectiveCallRisk(definition, call.Arguments),
	}
	permissionCtx, permissionCancel := s.permissionContext(ctx)
	defer permissionCancel()

	if guardErr := s.guardCall(permissionCtx, request); guardErr != nil {
		resolution := permission.Resolution{
			Action: permission.ActionDeny,
			Reason: guardErr.Error(),
		}
		s.observePermission(ctx, telemetry, resolution)
		permissionErr := fmt.Errorf("%w: %s", ErrPermissionDenied, resolution.Reason)
		result := tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Denied:   true,
			Failure:  &tool.Failure{Code: tool.ErrorCodePermissionDenied, Message: permissionErr.Error()},
		}
		s.observeCallResult(ctx, telemetry, result, permissionErr)
		return result, permissionErr
	}

	resolution, authorizeErr := s.authorize(permissionCtx, request)
	s.observePermission(ctx, telemetry, resolution)
	if authorizeErr != nil {
		result := tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Denied:   true,
			Failure:  tool.FailureFromError(authorizeErr),
		}
		s.observeCallResult(ctx, telemetry, result, authorizeErr)
		return result, authorizeErr
	}
	if resolution.Action != permission.ActionAllow {
		permissionErr := fmt.Errorf("%w: %s", ErrPermissionDenied, resolution.Reason)
		result := tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Denied:   true,
			Failure:  &tool.Failure{Code: tool.ErrorCodePermissionDenied, Message: permissionErr.Error()},
		}
		s.observeCallResult(ctx, telemetry, result, permissionErr)
		return result, permissionErr
	}
	if resolution.Scope == permission.GrantScopeSession {
		s.rememberGrant(request.Key())
	}

	executionCtx, executionCancel := s.executionContext(ctx, definition)
	defer executionCancel()
	result, err := executeHandler(executionCtx, handler, call)
	if result.CallID == "" {
		result.CallID = call.ID
	}
	if result.ToolName == "" {
		result.ToolName = call.Name
	}
	if err != nil {
		wrappedErr := fmt.Errorf("execute %s: %w", call.Name, err)
		if result.Failure == nil {
			result.Failure = tool.FailureFromError(wrappedErr)
		}
		s.observeCallResult(ctx, telemetry, result, wrappedErr)
		return result, wrappedErr
	}
	if len(definition.OutputSchema) > 0 {
		validationErr := sdk.ValidateToolOutput(sdk.Tool{Name: definition.Name, OutputSchema: definition.OutputSchema}, result.StructuredOutput)
		if validationErr != nil {
			outputErr := tool.NewToolError(tool.ErrorCodeInvalidOutput, fmt.Sprintf("tool %q returned structured output that does not match its schema", call.Name))
			result.Failure = tool.FailureFromError(outputErr)
			s.observeCallResult(ctx, telemetry, result, outputErr)
			return result, outputErr
		}
	}
	s.observeCallResult(ctx, telemetry, result, nil)
	return result, nil
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

func executeHandler(ctx context.Context, handler tool.Handler, call tool.Call) (result tool.Result, err error) {
	defer func() {
		if recover() != nil {
			result = tool.Result{}
			err = errors.New("tool handler panicked")
		}
	}()
	return handler.Execute(ctx, call)
}

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
	if (request.ToolKind == permission.ToolTask || request.ToolKind == permission.ToolAgent) && (mode == permission.ModeAsk || mode == permission.ModeAuto) {
		reason := "structured task metadata is allowed"
		if request.ToolKind == permission.ToolAgent {
			reason = "subagent orchestration is allowed"
		}
		return permission.Resolution{Action: permission.ActionAllow, Reason: reason}, nil
	}
	if granted && request.Risk == tool.CommandRiskNormal {
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
		if request.Risk != tool.CommandRiskNormal {
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
