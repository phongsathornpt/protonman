// Package toolcall orchestrates permission checks and tool execution.
package toolcall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"strings"
	"sync"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
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
// WithWorkspaceMutationGate serializes mutating calls that target the same workspace instance.
func WithWorkspaceMutationGate(ws *workspace.Workspace) Option {
	return func(service *Service) error {
		if ws == nil {
			return fmt.Errorf("%w: workspace mutation gate requires a workspace", ErrInvalidService)
		}
		service.mutationWorkspace = ws
		return nil
	}
}

func WithExecutionTimeout(timeout time.Duration) Option {
	return func(service *Service) error {
		if timeout < 0 {
			return fmt.Errorf("%w: execution timeout cannot be negative", ErrInvalidService)
		}
		service.executionTimeout = timeout
		return nil
	}
}

type compiledToolValidators struct {
	input  *sdk.ToolSchemaValidator
	output *sdk.ToolSchemaValidator
}

type compiledValidatorRegistry interface {
	CompiledValidators(name string) (input, output *sdk.ToolSchemaValidator, ok bool)
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
	mutationWorkspace *workspace.Workspace
	validators        map[string]compiledToolValidators
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
		validators:        make(map[string]compiledToolValidators),
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(service); err != nil {
			return nil, err
		}
	}
	for _, definition := range registry.Definitions() {
		validators, err := validatorsForRegistry(registry, definition)
		if err != nil {
			return nil, fmt.Errorf("%w: compile schema contract for %q: %v", ErrInvalidService, definition.Name, err)
		}
		service.validators[definition.Name] = validators
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
	validators := make(map[string]compiledToolValidators, len(s.validators))
	for name, compiled := range s.validators {
		validators[name] = compiled
	}
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
		mutationWorkspace: s.mutationWorkspace,
		validators:        validators,
	}
}

// CloneWithRegistry creates independent permission state over a replacement
// registry while preserving the service policy and runtime options. Stateful
// session tools use this to bind handlers to one durable session aggregate.
func (s *Service) CloneWithRegistry(registry tool.Registry) (*Service, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: source service is required", ErrInvalidService)
	}
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrInvalidService)
	}
	s.mu.RLock()
	policy := s.policy
	observer := s.observer
	mode := s.mode
	prompt := s.prompt
	guard := s.guard
	permissionTimeout := s.permissionTimeout
	executionTimeout := s.executionTimeout
	mutationWorkspace := s.mutationWorkspace
	s.mu.RUnlock()
	clone, err := NewService(registry, policy,
		WithMode(mode),
		WithPermissionTimeout(permissionTimeout),
		WithExecutionTimeout(executionTimeout),
	)
	if err != nil {
		return nil, err
	}
	clone.observer = observer
	clone.prompt = prompt
	clone.guard = guard
	clone.mutationWorkspace = mutationWorkspace
	return clone, nil
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

// AddRule dynamically appends a static rule to the active permission policy.
func (s *Service) AddRule(rule permission.Rule) error {
	if s == nil || s.policy == nil {
		return ErrInvalidService
	}
	return s.policy.AddRule(rule)
}

// Policy returns the compiled static permission policy used by the service.
func (s *Service) Policy() *permission.Policy {
	if s == nil {
		return nil
	}
	return s.policy
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
	return s.call(ctx, call, 0)
}

func (s *Service) call(ctx context.Context, call tool.Call, recoveryDepth int) (tool.Result, error) {
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
	call.Arguments = tool.NormalizeArguments(definition, call.Arguments)
	telemetry.call.Arguments = append(json.RawMessage(nil), call.Arguments...)
	telemetry.toolKind = definition.Kind
	validators, validatorErr := s.validatorsFor(definition)
	if validatorErr != nil {
		contractErr := tool.WrapToolError(tool.ErrorCodeExecution, fmt.Sprintf("tool %q has an invalid schema contract", call.Name), validatorErr)
		result := tool.Result{CallID: call.ID, ToolName: call.Name, Failure: tool.FailureFromError(contractErr)}
		s.observeCallResult(ctx, telemetry, result, contractErr)
		return result, contractErr
	}
	if validators.input != nil {
		validationErr := validators.input.Validate(call.Arguments)
		if validationErr != nil {
			inputErr := tool.WrapToolError(tool.ErrorCodeInvalidArguments, fmt.Sprintf("tool %q arguments do not match its input schema", call.Name), validationErr).WithDiagnostic(validationErr.Error())
			result := tool.Result{CallID: call.ID, ToolName: call.Name, Failure: tool.FailureFromError(inputErr)}
			s.observeCallResult(ctx, telemetry, result, inputErr)
			return result, inputErr
		}
	}
	detail := permissionDetail(definition, call.Arguments)
	if provider, ok := handler.(tool.DetailProvider); ok {
		if custom := strings.TrimSpace(provider.PermissionDetail(call.Arguments)); custom != "" {
			detail = custom
		}
	}
	request := permission.Request{
		CallID:    call.ID,
		ToolName:  call.Name,
		ToolKind:  definition.Kind,
		Detail:    detail,
		Arguments: append(json.RawMessage(nil), call.Arguments...),
		Risk:      tool.EffectiveCallRisk(definition, call.Arguments),
		Effect:    tool.EffectiveCallEffect(definition, call.Arguments),
		Scope:     tool.EffectiveCallScope(definition, call.Arguments),
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
	if resolution.Scope == permission.GrantScopeSession && permission.SessionGrantEligible(request) {
		s.rememberGrant(request.Key())
	}

	releaseMutation := func() {}
	if s.mutationWorkspace != nil && tool.EffectiveCallMutability(definition, call.Arguments) == tool.MutabilityMutating {
		var gateErr error
		releaseMutation, gateErr = s.mutationWorkspace.AcquireMutation(ctx)
		if gateErr != nil {
			result := tool.Result{CallID: call.ID, ToolName: call.Name, Failure: tool.FailureFromError(gateErr)}
			s.observeCallResult(ctx, telemetry, result, gateErr)
			return result, gateErr
		}
		defer releaseMutation()
	}

	executionCtx, executionCancel := s.executionContext(ctx, definition)
	defer executionCancel()
	result, err := executeHandler(executionCtx, handler, call)
	if err != nil {
		if recoveredResult, recoveredErr, recovered := s.recoverCall(executionCtx, telemetry, handler, definition, validators, call, err, recoveryDepth); recovered {
			result, err = recoveredResult, recoveredErr
		}
	}
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
	if validators.output != nil {
		validationErr := validators.output.Validate(result.StructuredOutput)
		if validationErr != nil {
			message := fmt.Sprintf("tool %q returned structured output that does not match its schema", call.Name)
			if definition.Kind == tool.KindMCP {
				message = fmt.Sprintf("MCP tool %q violated its declared output schema", call.Name)
				if provider, ok := handler.(tool.ContractDiagnosticProvider); ok {
					diagnostic := provider.ContractDiagnostic()
					expected := "unspecified"
					if value, ok := definition.OutputSchema["type"].(string); ok && strings.TrimSpace(value) != "" {
						expected = value
					}
					message += fmt.Sprintf(" (server=%s generation=%d schema=%s expected=%s actual=%s)",
						diagnostic.Source, diagnostic.CatalogGeneration, diagnostic.SchemaFingerprint, expected, structuredJSONType(result.StructuredOutput))
				}
			}
			outputErr := tool.WrapToolError(tool.ErrorCodeInvalidOutput, message, validationErr)
			result.Failure = tool.FailureFromError(outputErr)
			s.observeCallResult(ctx, telemetry, result, outputErr)
			return result, outputErr
		}
	}
	s.observeCallResult(ctx, telemetry, result, nil)
	return result, nil
}

func (s *Service) recoverCall(ctx context.Context, telemetry callTelemetry, handler tool.Handler, definition tool.Definition, validators compiledToolValidators, call tool.Call, err error, recoveryDepth int) (tool.Result, error, bool) {
	failure := tool.FailureFromError(err)
	if failure == nil || failure.Recovery == nil {
		return tool.Result{}, nil, false
	}
	switch failure.Recovery.Action {
	case tool.RecoveryRestartPagination:
		return s.recoverPagination(ctx, telemetry, handler, definition, validators, call, failure.Recovery)
	case tool.RecoveryUseDedicatedTool:
		return s.recoverDedicatedTool(ctx, telemetry, call, failure.Recovery, recoveryDepth)
	default:
		return tool.Result{}, nil, false
	}
}

func (s *Service) recoverPagination(ctx context.Context, telemetry callTelemetry, handler tool.Handler, definition tool.Definition, validators compiledToolValidators, call tool.Call, recovery *tool.Recovery) (tool.Result, error, bool) {
	if recovery.Tool != definition.Name || tool.EffectiveMutability(definition) != tool.MutabilityReadOnly {
		return tool.Result{}, nil, false
	}
	recoveryArgs := tool.NormalizeArguments(definition, recovery.Arguments)
	if validators.input != nil {
		if validationErr := validators.input.Validate(recoveryArgs); validationErr != nil {
			return tool.Result{}, nil, false
		}
	}
	s.observeRecovery(ctx, telemetry, EventRecoveryAttempted, recovery.Action, nil)
	retry := call
	retry.Arguments = recoveryArgs
	result, retryErr := executeHandler(ctx, handler, retry)
	if retryErr != nil {
		s.observeRecovery(ctx, telemetry, EventRecoveryFailed, recovery.Action, retryErr)
	} else {
		s.observeRecovery(ctx, telemetry, EventRecoverySucceeded, recovery.Action, nil)
	}
	return result, retryErr, true
}

func (s *Service) recoverDedicatedTool(ctx context.Context, telemetry callTelemetry, call tool.Call, recovery *tool.Recovery, recoveryDepth int) (tool.Result, error, bool) {
	if recoveryDepth > 0 || recovery == nil || strings.TrimSpace(recovery.Tool) == "" || recovery.Tool == call.Name {
		return tool.Result{}, nil, false
	}
	handler, ok := s.registry.Lookup(recovery.Tool)
	if !ok {
		return tool.Result{}, nil, false
	}
	definition := handler.Definition()
	recoveryArgs := tool.NormalizeArguments(definition, recovery.Arguments)
	if tool.EffectiveCallMutability(definition, recoveryArgs) != tool.MutabilityReadOnly ||
		tool.EffectiveCallSafety(definition, recoveryArgs).Boundary != tool.BoundaryPolicyWorkspaceRead {
		return tool.Result{}, nil, false
	}
	validators, err := s.validatorsFor(definition)
	if err != nil || validators.input != nil && validators.input.Validate(recoveryArgs) != nil {
		return tool.Result{}, nil, false
	}
	recoveryCall, err := tool.NewCall(call.ID+":recovery", definition.Name, recoveryArgs)
	if err != nil {
		return tool.Result{}, nil, false
	}
	s.observeRecovery(ctx, telemetry, EventRecoveryAttempted, recovery.Action, nil)
	result, recoveryErr := s.call(ctx, recoveryCall, recoveryDepth+1)
	if recoveryErr != nil {
		s.observeRecovery(ctx, telemetry, EventRecoveryFailed, recovery.Action, recoveryErr)
	} else {
		s.observeRecovery(ctx, telemetry, EventRecoverySucceeded, recovery.Action, nil)
	}
	result.CallID = call.ID
	result.ToolName = call.Name
	return result, recoveryErr, true
}

func validatorsForRegistry(registry tool.Registry, definition tool.Definition) (compiledToolValidators, error) {
	if cached, ok := registry.(compiledValidatorRegistry); ok {
		input, output, found := cached.CompiledValidators(definition.Name)
		if found {
			return compiledToolValidators{input: input, output: output}, nil
		}
	}
	return compileDefinitionValidators(definition)
}

func structuredJSONType(raw json.RawMessage) string {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return "missing"
	}
	switch value[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		if value[0] == '-' || value[0] >= '0' && value[0] <= '9' {
			return "number"
		}
		return "invalid"
	}
}

func compileDefinitionValidators(definition tool.Definition) (compiledToolValidators, error) {
	sdkTool := sdk.Tool{Name: definition.Name, Description: definition.Description, InputSchema: definition.InputSchema, OutputSchema: definition.OutputSchema}
	input, err := sdk.CompileToolInputValidator(sdkTool)
	if err != nil {
		return compiledToolValidators{}, err
	}
	output, err := sdk.CompileToolOutputValidator(sdkTool)
	if err != nil {
		return compiledToolValidators{}, err
	}
	return compiledToolValidators{input: input, output: output}, nil
}

func (s *Service) validatorsFor(definition tool.Definition) (compiledToolValidators, error) {
	s.mu.RLock()
	validators, ok := s.validators[definition.Name]
	s.mu.RUnlock()
	if ok {
		return validators, nil
	}
	compiled, err := validatorsForRegistry(s.registry, definition)
	if err != nil {
		return compiledToolValidators{}, err
	}
	s.mu.Lock()
	if existing, exists := s.validators[definition.Name]; exists {
		compiled = existing
	} else {
		s.validators[definition.Name] = compiled
	}
	s.mu.Unlock()
	return compiled, nil
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
