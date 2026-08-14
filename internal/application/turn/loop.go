// Package turn coordinates one model response stream with permission-aware
// tool execution.
package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/projectTHORN/proton/internal/application/toolcall"
	"github.com/projectTHORN/proton/internal/domain/model"
	"github.com/projectTHORN/proton/internal/domain/permission"
	"github.com/projectTHORN/proton/internal/domain/tool"
)

const (
	defaultMaxRounds       = 8
	defaultMaxParallelRead = 4
)

var (
	// ErrInvalidLoop indicates that the loop cannot be constructed or started.
	ErrInvalidLoop = errors.New("invalid model/tool loop")
	// ErrMaxRounds indicates that a model kept requesting tools without a final response.
	ErrMaxRounds = errors.New("model/tool round limit exceeded")
)

// EventKind identifies progress emitted by the application loop.
type EventKind string

const (
	// EventTextDelta forwards streamed model text.
	EventTextDelta EventKind = "text_delta"
	// EventToolCall reports a validated tool call before permission evaluation.
	EventToolCall EventKind = "tool_call"
	// EventToolResult reports the terminal result of a permission-aware call.
	EventToolResult EventKind = "tool_result"
	// EventCompleted marks a final model response with no further tool calls.
	EventCompleted EventKind = "completed"
	// EventFailed reports a terminal loop failure.
	EventFailed EventKind = "failed"
)

// Event is one progress or terminal notification from a model/tool turn.
type Event struct {
	Kind    EventKind
	Round   int
	Text    string
	Call    tool.Call
	Result  tool.Result
	Message model.Message
	Err     error
}

// Sink receives loop events in emission order.
type Sink func(context.Context, Event) error

// Result is the final assistant response from a completed turn.
type Result struct {
	Message model.Message
	Rounds  int
}

// Option configures a Loop.
type Option func(*Loop) error

// WithMaxRounds bounds model responses that can request more tools.
func WithMaxRounds(rounds int) Option {
	return func(loop *Loop) error {
		if rounds <= 0 {
			return fmt.Errorf("%w: max rounds must be positive", ErrInvalidLoop)
		}
		loop.maxRounds = rounds
		return nil
	}
}

// WithRoundTimeout bounds one model response and its tool calls.
func WithRoundTimeout(timeout time.Duration) Option {
	return func(loop *Loop) error {
		if timeout < 0 {
			return fmt.Errorf("%w: round timeout cannot be negative", ErrInvalidLoop)
		}
		loop.roundTimeout = timeout
		return nil
	}
}

// WithToolTimeout bounds one individual tool call. Zero disables this bound.
func WithToolTimeout(timeout time.Duration) Option {
	return func(loop *Loop) error {
		if timeout < 0 {
			return fmt.Errorf("%w: tool timeout cannot be negative", ErrInvalidLoop)
		}
		loop.toolTimeout = timeout
		return nil
	}
}

// WithMaxParallelReads bounds the read-only worker pool. One disables parallel dispatch.
func WithMaxParallelReads(limit int) Option {
	return func(loop *Loop) error {
		if limit <= 0 {
			return fmt.Errorf("%w: max parallel reads must be positive", ErrInvalidLoop)
		}
		loop.maxParallelReads = limit
		return nil
	}
}

// Loop coordinates model streaming and permission-aware tool dispatch.
type Loop struct {
	client           model.Client
	tools            *toolcall.Service
	maxRounds        int
	roundTimeout     time.Duration
	toolTimeout      time.Duration
	maxParallelReads int
}

// NewLoop creates a provider-neutral model/tool execution loop.
func NewLoop(client model.Client, tools *toolcall.Service, options ...Option) (*Loop, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: model client is required", ErrInvalidLoop)
	}
	if tools == nil {
		return nil, fmt.Errorf("%w: tool-call service is required", ErrInvalidLoop)
	}
	loop := &Loop{
		client:           client,
		tools:            tools,
		maxRounds:        defaultMaxRounds,
		maxParallelReads: defaultMaxParallelRead,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(loop); err != nil {
			return nil, err
		}
	}
	return loop, nil
}

// Run executes model responses until one has no tool calls or the round bound is reached.
func (l *Loop) Run(ctx context.Context, messages []model.Message, sink Sink) (Result, error) {
	if l == nil {
		return Result{}, fmt.Errorf("%w: loop is required", ErrInvalidLoop)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("start model/tool loop: %w", err)
	}
	if sink == nil {
		sink = func(context.Context, Event) error { return nil }
	}

	history := model.CloneMessages(messages)
	for round := 1; round <= l.maxRounds; round++ {
		request := model.Request{
			Messages: model.CloneMessages(history),
			Tools:    l.tools.Definitions(),
		}
		if err := request.Validate(); err != nil {
			return l.fail(ctx, sink, round, err)
		}
		assistant, executions, err := l.runRound(ctx, round, request, sink)
		if err != nil {
			return Result{}, err
		}
		history = append(history, assistant)
		if len(executions) == 0 {
			result := Result{
				Message: assistant,
				Rounds:  round,
			}
			if err := emit(ctx, sink, Event{
				Kind:    EventCompleted,
				Round:   round,
				Message: assistant,
			}); err != nil {
				return Result{}, err
			}
			return result, nil
		}

		for _, execution := range executions {
			toolResult := execution.result
			content, err := json.Marshal(toolResult)
			if err != nil {
				return l.fail(
					ctx,
					sink,
					round,
					fmt.Errorf("encode tool result %q: %w", execution.call.Name, err),
				)
			}
			history = append(history, model.Message{
				Role:       model.RoleTool,
				Content:    string(content),
				ToolCallID: execution.call.ID,
				ToolName:   execution.call.Name,
			})
		}
	}

	return l.fail(ctx, sink, l.maxRounds, fmt.Errorf("%w: %d rounds", ErrMaxRounds, l.maxRounds))
}

type executedCall struct {
	call   tool.Call
	result tool.Result
	err    error
}

func (l *Loop) runRound(
	parent context.Context,
	round int,
	request model.Request,
	sink Sink,
) (model.Message, []executedCall, error) {
	roundContext, cancel := l.newRoundContext(parent)
	defer cancel()

	assistant, requestedCalls, err := l.streamRound(roundContext, round, request, sink)
	if err != nil {
		return model.Message{}, nil, err
	}
	if len(requestedCalls) == 0 {
		return assistant, []executedCall{}, nil
	}

	calls := make([]tool.Call, 0, len(requestedCalls))
	for _, requestedCall := range requestedCalls {
		call, err := tool.NewCall(
			requestedCall.ID,
			requestedCall.Name,
			requestedCall.Arguments,
		)
		if err != nil {
			return model.Message{}, nil, fmt.Errorf("translate model tool call: %w", err)
		}
		if err := emit(roundContext, sink, Event{
			Kind:  EventToolCall,
			Round: round,
			Call:  call,
		}); err != nil {
			return model.Message{}, nil, err
		}
		calls = append(calls, call)
	}

	executions := l.executeCalls(roundContext, calls)
	if err := roundContext.Err(); err != nil {
		return model.Message{}, nil, fmt.Errorf("execute model round %d: %w", round, err)
	}
	for _, execution := range executions {
		if err := emit(roundContext, sink, Event{
			Kind:   EventToolResult,
			Round:  round,
			Call:   execution.call,
			Result: execution.result,
			Err:    execution.err,
		}); err != nil {
			return model.Message{}, nil, err
		}
	}
	return assistant, executions, nil
}

func (l *Loop) newRoundContext(parent context.Context) (context.Context, context.CancelFunc) {
	if l.roundTimeout > 0 {
		return context.WithTimeout(parent, l.roundTimeout)
	}
	return context.WithCancel(parent)
}

func (l *Loop) executeCalls(ctx context.Context, calls []tool.Call) []executedCall {
	if l.canRunConcurrently(calls) {
		return l.executeConcurrent(ctx, calls)
	}
	results := make([]executedCall, 0, len(calls))
	for _, call := range calls {
		if err := ctx.Err(); err != nil {
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
		if !ok || !readOnlyKind(definition.Kind) {
			return false
		}
	}
	return true
}

func readOnlyKind(kind tool.Kind) bool {
	return kind == tool.KindRead || kind == tool.KindGrep
}

func (l *Loop) executeConcurrent(ctx context.Context, calls []tool.Call) []executedCall {
	workerCount := l.maxParallelReads
	if workerCount > len(calls) {
		workerCount = len(calls)
	}

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
	for index := 0; index < workerCount; index++ {
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

func (l *Loop) streamRound(
	ctx context.Context,
	round int,
	request model.Request,
	sink Sink,
) (model.Message, []model.ToolCall, error) {
	stream, err := l.client.Stream(ctx, request)
	if err != nil {
		return model.Message{}, nil, fmt.Errorf("stream model round %d: %w", round, err)
	}
	if stream == nil {
		return model.Message{}, nil, fmt.Errorf("stream model round %d: nil stream", round)
	}
	assistant, calls, streamErr := consumeStream(ctx, round, stream, sink)
	closeErr := stream.Close()
	if streamErr != nil {
		return model.Message{}, nil, streamErr
	}
	if closeErr != nil {
		return model.Message{}, nil, fmt.Errorf("close model stream round %d: %w", round, closeErr)
	}
	return assistant, calls, nil
}

func consumeStream(
	ctx context.Context,
	round int,
	stream model.Stream,
	sink Sink,
) (model.Message, []model.ToolCall, error) {
	var text strings.Builder
	calls := make([]model.ToolCall, 0)
	for {
		event, err := stream.Next(ctx)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return model.Message{}, nil, fmt.Errorf("read model stream round %d: %w", round, err)
		}
		if err := event.Validate(); err != nil {
			return model.Message{}, nil, fmt.Errorf("validate model stream round %d: %w", round, err)
		}
		switch event.Kind {
		case model.EventTextDelta:
			text.WriteString(event.Text)
			if err := emit(ctx, sink, Event{
				Kind:  EventTextDelta,
				Round: round,
				Text:  event.Text,
			}); err != nil {
				return model.Message{}, nil, err
			}
		case model.EventToolCall:
			call := event.ToolCall
			call.Arguments = append(json.RawMessage{}, call.Arguments...)
			calls = append(calls, call)
		case model.EventDone:
			return model.Message{
				Role:      model.RoleAssistant,
				Content:   text.String(),
				ToolCalls: calls,
			}, calls, nil
		}
	}
	return model.Message{
		Role:      model.RoleAssistant,
		Content:   text.String(),
		ToolCalls: calls,
	}, calls, nil
}

func (l *Loop) fail(ctx context.Context, sink Sink, round int, err error) (Result, error) {
	if emitErr := emit(ctx, sink, Event{
		Kind:  EventFailed,
		Round: round,
		Err:   err,
	}); emitErr != nil {
		return Result{}, emitErr
	}
	return Result{}, err
}

func emit(ctx context.Context, sink Sink, event Event) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("emit model/tool event: %w", err)
	}
	if err := sink(ctx, event); err != nil {
		return fmt.Errorf("emit model/tool event: %w", err)
	}
	return nil
}
