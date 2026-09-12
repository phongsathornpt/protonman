package toolcall

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func (s *Service) recoverCall(ctx context.Context, telemetry callTelemetry, handler tool.Handler, definition tool.Definition, validators compiledToolValidators, call tool.Call, err error, recoveryDepth int) (tool.Result, error, bool) {
	failure := tool.FailureFromError(err)
	if failure == nil || failure.Recovery == nil {
		return tool.Result{}, nil, false
	}
	switch failure.Recovery.Action {
	case tool.RecoveryRestartPagination:
		return s.recoverPagination(ctx, telemetry, handler, definition, validators, call, failure.Recovery)
	case tool.RecoveryRefreshResource, tool.RecoveryDiscoverResource:
		return s.recoverResourceEvidence(ctx, telemetry, call, err, failure.Recovery, recoveryDepth)
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

func (s *Service) recoverResourceEvidence(ctx context.Context, telemetry callTelemetry, call tool.Call, originalErr error, recovery *tool.Recovery, recoveryDepth int) (tool.Result, error, bool) {
	if recoveryDepth > 0 || recovery == nil || strings.TrimSpace(recovery.Tool) == "" {
		return tool.Result{}, nil, false
	}
	handler, ok := s.registry.Lookup(recovery.Tool)
	if !ok {
		return tool.Result{}, nil, false
	}
	definition := handler.Definition()
	recoveryArgs := tool.NormalizeArguments(definition, recovery.Arguments)
	if tool.EffectiveCallMutability(definition, recoveryArgs) != tool.MutabilityReadOnly {
		return tool.Result{}, nil, false
	}
	validators, err := s.validatorsFor(definition)
	if err != nil || (validators.input != nil && validators.input.Validate(recoveryArgs) != nil) {
		return tool.Result{}, nil, false
	}
	recoveryCall, err := tool.NewCall(call.ID+":refresh", definition.Name, recoveryArgs)
	if err != nil {
		return tool.Result{}, nil, false
	}
	s.observeRecovery(ctx, telemetry, EventRecoveryAttempted, recovery.Action, nil)
	refreshed, refreshErr := s.call(ctx, recoveryCall, recoveryDepth+1)
	if refreshErr != nil || refreshed.Failure != nil || refreshed.Denied {
		if refreshErr == nil {
			refreshErr = errors.New("resource refresh failed")
		}
		s.observeRecovery(ctx, telemetry, EventRecoveryFailed, recovery.Action, refreshErr)
		return tool.Result{}, nil, false
	}
	s.observeRecovery(ctx, telemetry, EventRecoverySucceeded, recovery.Action, nil)
	failure := tool.FailureFromError(originalErr)
	if failure == nil {
		return tool.Result{}, nil, false
	}
	payload := refreshed.ModelPayload()
	failure.RecoveryEvidence = &tool.RecoveryEvidence{
		Action:           recovery.Action,
		Tool:             definition.Name,
		Output:           payload.Output,
		StructuredOutput: append(json.RawMessage(nil), payload.StructuredOutput...),
		SHA256:           payload.SHA256,
		Truncated:        payload.Truncated,
		Pagination:       payload.Pagination,
	}
	return tool.Result{CallID: call.ID, ToolName: call.Name, Failure: failure}, originalErr, true
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
	if err != nil || (validators.input != nil && validators.input.Validate(recoveryArgs) != nil) {
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
