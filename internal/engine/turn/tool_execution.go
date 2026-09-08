package turn

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/phongsathornpt/proton/internal/core/permission"
	"github.com/phongsathornpt/proton/internal/core/tool"
)

func (l *Loop) executeCalls(ctx context.Context, calls []tool.Call) []executedCall {
	concurrent := l.canRunConcurrently(calls)
	if concurrent {
		slog.DebugContext(ctx, "tool dispatch mode",
			"call_count", len(calls),
			"mode", "concurrent",
		)
		return l.executeConcurrent(ctx, calls)
	}
	slog.DebugContext(ctx, "tool dispatch mode",
		"call_count", len(calls),
		"mode", "serial",
	)
	results := make([]executedCall, 0, len(calls))
	for _, call := range calls {
		if err := ctx.Err(); err != nil {
			slog.DebugContext(ctx, "tool dispatch skipped",
				"call_id", call.ID,
				"tool_name", call.Name,
				"error_type", fmt.Sprintf("%T", err),
			)
			results = append(results, canceledCall(call, err))
			continue
		}
		results = append(results, l.executeOne(ctx, call))
	}
	return results
}

func (l *Loop) canRunConcurrently(calls []tool.Call) bool {
	if len(calls) < 2 || l.maxParallelReads < 2 {
		return false
	}
	if l.tools.Mode() != permission.ModeAlwaysApprove {
		return false
	}
	publishedDefinitions := l.tools.Definitions()
	definitions := make(map[string]tool.Definition, len(publishedDefinitions))
	for _, definition := range publishedDefinitions {
		definitions[definition.Name] = definition
	}
	for _, call := range calls {
		definition, ok := definitions[call.Name]
		if !ok || !readOnlyDefinition(definition) {
			return false
		}
	}
	return true
}

func readOnlyDefinition(definition tool.Definition) bool {
	return tool.EffectiveMutability(definition) == tool.MutabilityReadOnly
}

func (l *Loop) executeConcurrent(ctx context.Context, calls []tool.Call) []executedCall {
	workerCount := min(l.maxParallelReads, len(calls))
	slog.DebugContext(ctx, "tool dispatch workers",
		"call_count", len(calls),
		"worker_count", workerCount,
	)

	type indexedCall struct {
		index int
		call  tool.Call
	}
	type indexedResult struct {
		index int
		call  executedCall
	}
	jobs := make(chan indexedCall)
	results := make(chan indexedResult, len(calls))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					results <- indexedResult{
						index: job.index,
						call:  l.executeOne(ctx, job.call),
					}
				}
			}
		}()
	}

sending:
	for index, call := range calls {
		select {
		case jobs <- indexedCall{index: index, call: call}:
		case <-ctx.Done():
			break sending
		}
	}
	close(jobs)
	workers.Wait()
	close(results)

	executions := make([]executedCall, len(calls))
	completed := make([]bool, len(calls))
	for result := range results {
		executions[result.index] = result.call
		completed[result.index] = true
	}
	if err := ctx.Err(); err != nil {
		for index, complete := range completed {
			if !complete {
				executions[index] = canceledCall(calls[index], err)
			}
		}
	}
	return executions
}

func (l *Loop) executeOne(ctx context.Context, call tool.Call) executedCall {
	startedAt := time.Now()
	slog.DebugContext(ctx, "tool execution started",
		"call_id", call.ID,
		"tool_name", call.Name,
		"argument_bytes", len(call.Arguments),
	)
	callContext := ctx
	cancel := func() {}
	if l.toolTimeout > 0 {
		callContext, cancel = context.WithTimeout(ctx, l.toolTimeout)
	}
	result, err := l.tools.Call(callContext, call)
	cancel()
	if err != nil && result.Failure == nil {
		result.Failure = tool.FailureFromError(err)
	}
	attrs := []any{
		"call_id", call.ID,
		"tool_name", call.Name,
		"duration_ms", time.Since(startedAt).Milliseconds(),
		"success", err == nil,
	}
	if result.Failure != nil {
		attrs = append(attrs, "error_code", result.Failure.Code)
	}
	slog.DebugContext(ctx, "tool execution finished", attrs...)
	return executedCall{
		call:   call,
		result: result,
		err:    err,
	}
}

func canceledCall(call tool.Call, err error) executedCall {
	return executedCall{
		call: call,
		result: tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Failure:  tool.FailureFromError(err),
		},
		err: err,
	}
}

func logExecutionSummary(ctx context.Context, round int, executions []executedCall) {
	failed := 0
	denied := 0
	for _, execution := range executions {
		if execution.err != nil {
			failed++
		}
		if execution.result.Denied {
			denied++
		}
	}
	slog.DebugContext(ctx, "tool dispatch finished",
		"round", round,
		"call_count", len(executions),
		"failed_count", failed,
		"denied_count", denied,
	)
}
