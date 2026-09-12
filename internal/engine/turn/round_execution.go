package turn

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/phongsathornpt/protonman/internal/base/contextutil"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type executedCall struct {
	call                tool.Call
	result              tool.Result
	err                 error
	suppressed          bool
	suppressionReason   string
	semanticFingerprint string
	repeatCount         int
	retryable           bool
	modelToolName       string
}

func (l *Loop) runRound(
	parent context.Context,
	round int,
	request sdk.Request,
	dispatch toolDispatchState,
	progress *progressGuard,
	resultBudget *toolResultBudget,
	sink Sink,
) (roundOutcome, error) {
	roundContext, cancel := l.newRoundContext(parent)
	defer cancel()

	assistant, requestedCalls, err := l.streamRound(roundContext, round, request, sink)
	if err != nil {
		slog.DebugContext(parent, "turn round model failed",
			"round", round,
			"error_type", fmt.Sprintf("%T", err),
		)
		return roundOutcome{}, err
	}
	seenIDs := make(map[string]struct{}, len(requestedCalls))
	for _, requestedCall := range requestedCalls {
		if _, exists := seenIDs[requestedCall.ID]; exists {
			return roundOutcome{}, fmt.Errorf("%w: %q", ErrDuplicateToolCall, requestedCall.ID)
		}
		seenIDs[requestedCall.ID] = struct{}{}
	}
	slog.DebugContext(parent, "turn round model completed",
		"round", round,
		"assistant_bytes", len(assistant.Content),
		"tool_calls", len(requestedCalls),
	)
	if len(requestedCalls) == 0 || !dispatch.enabled() {
		slog.DebugContext(parent, "turn round has no tool dispatch",
			"round", round,
			"requested_tool_calls", len(requestedCalls),
			"published_tools", len(request.Tools),
			"dispatch_enabled", dispatch.enabled(),
			"dispatch_disabled_reason", dispatch.reason,
		)
		return roundOutcome{
			assistant:  assistant,
			executions: []executedCall{},
			dispatch:   dispatch,
		}, nil
	}
	if dispatch.remainingSafetyCalls > 0 && len(requestedCalls) > dispatch.remainingSafetyCalls {
		slog.DebugContext(parent, "turn tool safety budget exceeded",
			"round", round,
			"requested_tool_calls", len(requestedCalls),
			"remaining_safety_calls", dispatch.remainingSafetyCalls,
		)
		return roundOutcome{
			assistant:  assistant,
			executions: []executedCall{},
			dispatch: toolDispatchState{
				reason: toolDispatchDisabledSafetyBudget,
			},
		}, nil
	}
	if len(request.Tools) == 0 {
		return roundOutcome{}, fmt.Errorf("%w: dispatch enabled without published tools", ErrInvalidLoop)
	}

	calls := make([]tool.Call, 0, len(requestedCalls))
	modelNames := make(map[string]string, len(requestedCalls))
	for _, requestedCall := range requestedCalls {
		canonicalName := dispatch.canonicalToolName(requestedCall.Name)
		call, err := tool.NewCall(
			requestedCall.ID,
			canonicalName,
			requestedCall.Arguments,
		)
		if err != nil {
			slog.DebugContext(roundContext, "turn tool-call translation failed",
				"round", round,
				"error_type", fmt.Sprintf("%T", err),
			)
			return roundOutcome{}, fmt.Errorf("translate model tool call: %w", err)
		}
		if err := emit(roundContext, sink, Event{
			Kind:  EventToolCall,
			Round: round,
			Call:  call,
		}); err != nil {
			slog.DebugContext(roundContext, "turn tool-call event failed",
				"round", round,
				"error_type", fmt.Sprintf("%T", err),
			)
			return roundOutcome{}, err
		}
		modelNames[call.ID] = requestedCall.Name
		calls = append(calls, call)
	}

	concurrent := l.canRunConcurrently(calls)
	slog.DebugContext(roundContext, "turn tool dispatch started",
		"round", round,
		"call_count", len(calls),
		"concurrent", concurrent,
	)
	executions := make([]executedCall, len(calls))
	pendingCalls := make([]tool.Call, 0, len(calls))
	pendingIndexes := make([]int, 0, len(calls))
	for index, call := range calls {
		suppressed, suppressErr := progress.suppress(call)
		if suppressErr != nil {
			return roundOutcome{}, suppressErr
		}
		if suppressed != nil {
			executions[index] = *suppressed
			executions[index].modelToolName = modelNames[call.ID]
			l.observeSuppression(roundContext, round, *suppressed)
			continue
		}
		pendingCalls = append(pendingCalls, call)
		pendingIndexes = append(pendingIndexes, index)
	}
	if len(pendingCalls) > 0 {
		dispatched := l.executeCalls(roundContext, pendingCalls)
		for index, execution := range dispatched {
			execution.modelToolName = modelNames[execution.call.ID]
			executions[pendingIndexes[index]] = execution
		}
	}
	if resultBudget != nil {
		executions = resultBudget.applyRound(executions)
	}
	logExecutionSummary(roundContext, round, executions)
	if err := roundContext.Err(); err != nil {
		slog.DebugContext(parent, "turn round cancelled",
			"round", round,
			"error_type", fmt.Sprintf("%T", err),
		)
		emitContext, emitCancel := contextutil.DetachedTimeout(roundContext, terminalEmitTimeout)
		defer emitCancel()
		for _, execution := range executions {
			if emitErr := emit(emitContext, sink, Event{
				Kind:   EventToolResult,
				Round:  round,
				Call:   execution.call,
				Result: execution.result,
				Err:    execution.err,
			}); emitErr != nil {
				slog.DebugContext(emitContext, "turn cancelled result event failed",
					"round", round,
					"error_type", fmt.Sprintf("%T", emitErr),
				)
				return roundOutcome{}, emitErr
			}
		}
		return roundOutcome{}, fmt.Errorf("execute model round %d: %w", round, err)
	}
	for _, execution := range executions {
		if err := emit(roundContext, sink, Event{
			Kind:   EventToolResult,
			Round:  round,
			Call:   execution.call,
			Result: execution.result,
			Err:    execution.err,
		}); err != nil {
			slog.DebugContext(roundContext, "turn tool-result event failed",
				"round", round,
				"error_type", fmt.Sprintf("%T", err),
			)
			return roundOutcome{}, err
		}
	}
	return roundOutcome{
		assistant:  assistant,
		executions: executions,
		dispatch:   dispatch,
	}, nil
}
