// Package toolcall orchestrates permission checks and tool execution.
package toolcall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
)

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
			Failure:  &tool.Failure{Code: tool.ErrorCodePermissionDenied, Message: resolution.Reason},
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

func executeHandler(ctx context.Context, handler tool.Handler, call tool.Call) (result tool.Result, err error) {
	defer func() {
		if recover() != nil {
			result = tool.Result{}
			err = errors.New("tool handler panicked")
		}
	}()
	return handler.Execute(ctx, call)
}
