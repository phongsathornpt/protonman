// Package turn coordinates one model response stream with permission-aware
// tool execution.
package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/projectTHORN/proton/internal/contextutil"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

const (
	terminalEmitTimeout       = runtimepolicy.TerminalEmitTimeout
	protectionObserverTimeout = runtimepolicy.ProtectionObserverTimeout

	// DefaultMaxRounds is the default maximum number of rounds per turn.
	DefaultMaxRounds = runtimepolicy.TurnMaxRounds
	// DefaultMaxToolCalls is the default cumulative maximum number of tool
	// calls per turn.
	DefaultMaxToolCalls = runtimepolicy.TurnMaxToolCalls
	// DefaultTurnTimeout bounds one complete model/tool turn.
	DefaultTurnTimeout = runtimepolicy.TurnTimeout
	// DefaultRoundTimeout bounds a turn round when callers do not provide a
	// stricter timeout.
	DefaultRoundTimeout    = runtimepolicy.RoundTimeout
	defaultMaxRounds       = DefaultMaxRounds
	defaultMaxToolCalls    = DefaultMaxToolCalls
	defaultMaxParallelRead = 4
	skillPromptMarker      = "<!-- proton:skill-catalog -->"
)

// MaxRoundsPrompt is injected when the turn reaches max rounds to compel a final synthesis response without tools.
const MaxRoundsPrompt = `CRITICAL - MAXIMUM TOOL ROUNDS REACHED

The maximum number of tool execution rounds allowed for this turn has been reached. Tools are disabled until next user input. Respond with text only.

STRICT REQUIREMENTS:
1. Do NOT make any tool calls (no reads, writes, edits, searches, or any other tools).
2. MUST provide a clear text response summarizing what was accomplished so far.
3. List any remaining tasks that were not completed.
4. Provide recommendations for what the user or next step should do.

Respond with text ONLY.`

// MaxRoundsFallback is used when a provider ignores MaxRoundsPrompt and still
// returns a tool call after dispatch has been disabled.
const MaxRoundsFallback = "I reached the maximum number of tool rounds before producing a final response. The last tool request was not executed. Review the work so far or start a new turn."

// MaxToolCallsPrompt is injected when the turn reaches the cumulative tool
// call limit to compel a final synthesis response without tools.
const MaxToolCallsPrompt = `CRITICAL - MAXIMUM TOOL CALLS REACHED

The maximum cumulative number of tool calls allowed for this turn has been reached. Tools are disabled until next user input. Respond with text only.

STRICT REQUIREMENTS:
1. Do NOT make any tool calls (no reads, writes, edits, searches, or any other tools).
2. MUST provide a clear text response summarizing what was accomplished so far.
3. List any remaining tasks that were not completed.
4. Provide recommendations for what the user or next step should do.

Respond with text ONLY.`

// MaxToolCallsFallback is used when a provider ignores MaxToolCallsPrompt or
// requests more calls than the remaining budget.
const MaxToolCallsFallback = "I reached the maximum number of tool calls before producing a final response. The last tool request was not executed. Review the work so far or start a new turn."

// NoProgressPrompt is injected when deterministic tool calls repeatedly return
// the same result without any intervening workspace mutation.
const NoProgressPrompt = `CRITICAL - TOOL LOOP DETECTED

Repeated tool calls produced the same result or retryable failure without making progress. Tools are disabled until next user input. Respond with text only.

STRICT REQUIREMENTS:
1. Do NOT repeat the same tool call or make any other tool calls.
2. Summarize what was learned from the existing tool results.
3. Explain any blocker or missing capability that prevented progress.
4. Provide the best final answer possible from the information already gathered.

Respond with text ONLY.`

// NoProgressFallback is used when a provider ignores NoProgressPrompt and still
// requests another tool call after a semantic loop was detected.
const NoProgressFallback = "I stopped a repeated tool loop because the same call kept producing the same result or failure without progress. The last tool request was not executed."

var (
	// ErrInvalidLoop indicates that the loop cannot be constructed or started.
	ErrInvalidLoop = errors.New("invalid model/tool loop")
	// ErrMaxRounds indicates that a model kept requesting tools without a final response.
	ErrMaxRounds = errors.New("model/tool round limit exceeded")
	// ErrEmptyResponse indicates that the provider completed without text or tool calls.
	ErrEmptyResponse = errors.New("model returned an empty response")
	// ErrDuplicateToolCall indicates that one model response reused a call ID.
	ErrDuplicateToolCall = errors.New("duplicate model tool call")
	// ErrToolDispatchUnavailable indicates that a model requested tools when no
	// tools were available for the current round.
	ErrToolDispatchUnavailable = errors.New("tool dispatch unavailable")
	// ErrUnresolvedToolCall indicates that a requested call had no execution result.
	ErrUnresolvedToolCall = errors.New("unresolved model tool call")
	// ErrUnsupportedModelCapability indicates that the active model cannot satisfy a turn requirement.
	ErrUnsupportedModelCapability = errors.New("unsupported model capability")
)

type toolDispatchReason string

const (
	toolDispatchEnabled            toolDispatchReason = "enabled"
	toolDispatchDisabledMaxRounds  toolDispatchReason = "max_rounds"
	toolDispatchDisabledMaxCalls   toolDispatchReason = "max_tool_calls"
	toolDispatchDisabledNoProgress toolDispatchReason = "no_progress"
	toolDispatchDisabledNoTools    toolDispatchReason = "no_tools"
	toolDispatchDisabledModelTools toolDispatchReason = "model_tools_unsupported"
)

type toolDispatchState struct {
	reason             toolDispatchReason
	remainingToolCalls int
}

func (s toolDispatchState) enabled() bool {
	return s.reason == toolDispatchEnabled
}

type roundOutcome struct {
	assistant  model.Message
	executions []executedCall
	dispatch   toolDispatchState
}

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

// Runner is the injectable model/tool loop used by TUI, headless, and ACP adapters.
type Runner interface {
	Run(context.Context, []model.Message, Sink) (Result, error)
}

// Result is the final assistant response from a completed turn.
type Result struct {
	Message model.Message
	Rounds  int
	// Messages contains the assistant/tool messages produced during this run.
	// It excludes caller-supplied history and generated system prompt material.
	Messages []model.Message
}

// Option configures a Loop.
type Option func(*Loop) error

// WithMaxRounds bounds model responses that can request more tools.
// A value of 0 disables this count bound; other turn bounds still apply.
func WithMaxRounds(rounds int) Option {
	return func(loop *Loop) error {
		if rounds < 0 {
			return fmt.Errorf("%w: max rounds cannot be negative", ErrInvalidLoop)
		}
		loop.maxRounds = rounds
		return nil
	}
}

// WithMaxToolCalls bounds the cumulative number of tool calls per turn.
// A value of 0 disables this count bound; other turn bounds still apply.
func WithMaxToolCalls(calls int) Option {
	return func(loop *Loop) error {
		if calls < 0 {
			return fmt.Errorf("%w: max tool calls cannot be negative", ErrInvalidLoop)
		}
		loop.maxToolCalls = calls
		return nil
	}
}

// WithMaxIdenticalNoProgressResults bounds repeated identical deterministic
// tool results before Proton forces a text-only synthesis round. Zero disables
// semantic no-progress detection.
func WithMaxIdenticalNoProgressResults(limit int) Option {
	return func(loop *Loop) error {
		if limit < 0 {
			return fmt.Errorf("%w: max identical no-progress results cannot be negative", ErrInvalidLoop)
		}
		loop.maxIdenticalNoProgressResults = limit
		return nil
	}
}

// WithTurnTimeout bounds one complete model/tool turn. Zero disables this
// bound, which is only valid when another global execution limit remains set.
func WithTurnTimeout(timeout time.Duration) Option {
	return func(loop *Loop) error {
		if timeout < 0 {
			return fmt.Errorf("%w: turn timeout cannot be negative", ErrInvalidLoop)
		}
		loop.turnTimeout = timeout
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

// WithSkillCatalog supplies discovered skills for progressive disclosure in model requests.
func WithSkillCatalog(skills []skill.CatalogItem) Option {
	return func(loop *Loop) error {
		loop.skills = append([]skill.CatalogItem{}, skills...)
		return nil
	}
}

// WithSkillRegistry supplies a skill registry for dynamic progressive disclosure and active skill injection.
func WithSkillRegistry(registry *skill.Registry) Option {
	return func(loop *Loop) error {
		loop.skillRegistry = registry
		return nil
	}
}

// Loop coordinates model streaming and permission-aware tool dispatch.
type Loop struct {
	languageModel                 sdk.LanguageModel
	tools                         *toolcall.Service
	maxRounds                     int
	maxToolCalls                  int
	maxIdenticalNoProgressResults int
	turnTimeout                   time.Duration
	roundTimeout                  time.Duration
	toolTimeout                   time.Duration
	maxParallelReads              int
	skills                        []skill.CatalogItem
	skillRegistry                 *skill.Registry
}

var _ Runner = (*Loop)(nil)

// NewLoop creates a model/tool loop that consumes proton-sdk directly.
func NewLoop(languageModel sdk.LanguageModel, tools *toolcall.Service, options ...Option) (*Loop, error) {
	if languageModel == nil {
		return nil, fmt.Errorf("%w: language model is required", ErrInvalidLoop)
	}
	if !languageModel.Capabilities().Streaming {
		return nil, fmt.Errorf("%w: streaming is required", ErrUnsupportedModelCapability)
	}
	if tools == nil {
		return nil, fmt.Errorf("%w: tool-call service is required", ErrInvalidLoop)
	}
	loop := &Loop{
		languageModel:                 languageModel,
		tools:                         tools,
		maxRounds:                     defaultMaxRounds,
		maxToolCalls:                  defaultMaxToolCalls,
		maxIdenticalNoProgressResults: defaultMaxIdenticalNoProgressResults,
		turnTimeout:                   DefaultTurnTimeout,
		roundTimeout:                  DefaultRoundTimeout,
		maxParallelReads:              defaultMaxParallelRead,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(loop); err != nil {
			return nil, err
		}
	}
	if loop.maxRounds == 0 && loop.maxToolCalls == 0 && loop.turnTimeout == 0 {
		return nil, fmt.Errorf("%w: at least one of max rounds, max tool calls, or turn timeout must be bounded", ErrInvalidLoop)
	}
	return loop, nil
}

// CloneWithTools creates a loop with the same model and execution settings but
// an independent tool-call service. It is used by session-oriented adapters
// that share a model client while keeping permission state isolated.
func (l *Loop) CloneWithTools(tools *toolcall.Service) (*Loop, error) {
	if l == nil {
		return nil, fmt.Errorf("%w: loop is required", ErrInvalidLoop)
	}
	if tools == nil {
		return nil, fmt.Errorf("%w: tool-call service is required", ErrInvalidLoop)
	}
	clone := *l
	clone.tools = tools
	clone.skills = append([]skill.CatalogItem(nil), l.skills...)
	return &clone, nil
}

func messagesContainImages(messages []model.Message) bool {
	for _, message := range messages {
		for _, part := range message.Parts {
			if part.Type == model.ContentPartImage {
				return true
			}
		}
	}
	return false
}

// Run executes model responses until one has no tool calls or the round bound is reached.
func (l *Loop) Run(ctx context.Context, messages []model.Message, sink Sink) (Result, error) {
	if l == nil {
		slog.DebugContext(ctx, "turn rejected", "reason", "nil_loop")
		return Result{}, fmt.Errorf("%w: loop is required", ErrInvalidLoop)
	}
	if err := ctx.Err(); err != nil {
		slog.DebugContext(ctx, "turn rejected",
			"reason", "context_already_done",
			"error_type", fmt.Sprintf("%T", err),
		)
		return Result{}, fmt.Errorf("start model/tool loop: %w", err)
	}
	turnContext, cancelTurn := l.newTurnContext(ctx)
	defer cancelTurn()
	ctx = turnContext
	if sink == nil {
		sink = func(context.Context, Event) error { return nil }
	}
	startedAt := time.Now()
	terminalReason := "unknown"
	roundsCompleted := 0
	defer func() {
		slog.DebugContext(ctx, "turn finished",
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"rounds", roundsCompleted,
			"terminal_reason", terminalReason,
		)
	}()
	toolCount := 0
	if l.tools != nil {
		toolCount = len(l.tools.Definitions())
	}
	slog.DebugContext(ctx, "turn started",
		"message_count", len(messages),
		"tool_count", toolCount,
		"max_rounds", l.maxRounds,
		"max_tool_calls", l.maxToolCalls,
	)

	caps := l.languageModel.Capabilities()
	if !caps.Vision && messagesContainImages(messages) {
		terminalReason = "vision_unsupported"
		return l.fail(ctx, sink, 0, fmt.Errorf("%w: model %q does not support vision input", ErrUnsupportedModelCapability, l.languageModel.ModelID()))
	}

	history := model.CloneMessages(messages)
	turnMessages := make([]model.Message, 0, 4)
	toolCallsUsed := 0
	progress := newProgressGuard(l.tools.Definitions(), l.maxIdenticalNoProgressResults)
	forceNoProgressSynthesis := false

	var catalogItems []skill.CatalogItem
	var activeSkills []skill.Skill

	if l.skillRegistry != nil {
		allSkills := l.skillRegistry.List()
		activeMap := make(map[string]bool)
		for _, name := range l.skillRegistry.ActivatedList() {
			activeMap[name] = true
		}
		for _, s := range allSkills {
			if activeMap[s.Name] {
				activeSkills = append(activeSkills, s)
			} else {
				catalogItems = append(catalogItems, s.ToCatalogItem())
			}
		}
	} else if len(l.skills) > 0 {
		catalogItems = l.skills
	}

	if len(catalogItems) > 0 || len(activeSkills) > 0 {
		section := skill.SystemPromptSection(catalogItems, activeSkills)
		if len(history) > 0 && history[0].Role == model.RoleSystem {
			base := history[0].Content
			if marker := strings.Index(base, skillPromptMarker); marker >= 0 {
				base = base[:marker]
			}
			history[0].Content = strings.TrimSpace(base + "\n\n" + skillPromptMarker + "\n" + section)
		} else {
			systemMsg := model.Message{
				Role:    model.RoleSystem,
				Content: skillPromptMarker + "\n" + section,
			}
			history = append([]model.Message{systemMsg}, history...)
		}
	}
	for round := 1; ; round++ {
		roundsCompleted = round
		isMaxRound := l.maxRounds > 0 && round >= l.maxRounds
		slog.DebugContext(ctx, "turn round started",
			"round", round,
			"max_round", isMaxRound,
			"history_messages", len(history),
		)

		var tools []tool.Definition
		reqMessages := model.CloneMessages(history)
		dispatch := toolDispatchState{reason: toolDispatchDisabledNoTools}

		if forceNoProgressSynthesis {
			tools = nil
			dispatch.reason = toolDispatchDisabledNoProgress
			reqMessages = append(reqMessages, model.Message{
				Role:    model.RoleSystem,
				Content: NoProgressPrompt,
			})
		} else if isMaxRound {
			tools = nil
			dispatch.reason = toolDispatchDisabledMaxRounds
			reqMessages = append(reqMessages, model.Message{
				Role:    model.RoleSystem,
				Content: MaxRoundsPrompt,
			})
		} else {
			tools = l.tools.Definitions()
			switch {
			case len(tools) > 0 && !caps.Tools:
				tools = nil
				dispatch.reason = toolDispatchDisabledModelTools
			case len(tools) == 0:
				dispatch.reason = toolDispatchDisabledNoTools
			case l.maxToolCalls > 0 && toolCallsUsed >= l.maxToolCalls:
				tools = nil
				dispatch.reason = toolDispatchDisabledMaxCalls
				reqMessages = append(reqMessages, model.Message{
					Role:    model.RoleSystem,
					Content: MaxToolCallsPrompt,
				})
			default:
				dispatch.reason = toolDispatchEnabled
				if l.maxToolCalls > 0 {
					dispatch.remainingToolCalls = l.maxToolCalls - toolCallsUsed
				}
			}
		}
		slog.DebugContext(ctx, "turn tool dispatch state",
			"round", round,
			"enabled", dispatch.enabled(),
			"reason", dispatch.reason,
			"published_tools", len(tools),
			"tool_calls_used", toolCallsUsed,
			"remaining_tool_calls", dispatch.remainingToolCalls,
		)

		sdkTools := make([]sdk.Tool, 0, len(tools))
		for _, definition := range tools {
			sdkTools = append(sdkTools, sdk.Tool{
				Name:        definition.Name,
				Description: definition.Description,
				InputSchema: definition.InputSchema,
				Dynamic:     definition.Kind == tool.KindMCP,
			})
		}
		request := sdk.Request{
			Messages: reqMessages,
			Tools:    sdkTools,
		}
		if err := request.Validate(); err != nil {
			terminalReason = "request_validation_failed"
			return l.fail(ctx, sink, round, err)
		}
		outcome, err := l.runRound(ctx, round, request, dispatch, progress, sink)
		if err != nil {
			terminalReason = "round_failed"
			return l.fail(ctx, sink, round, err)
		}
		assistant := outcome.assistant
		executions := outcome.executions
		maxRoundFallback := false
		maxToolCallsFallback := false
		noProgressFallback := false
		if len(assistant.ToolCalls) > 0 && !outcome.dispatch.enabled() {
			switch outcome.dispatch.reason {
			case toolDispatchDisabledMaxRounds:
				assistant, err = finalizeMaxRoundToolCallResponse(ctx, sink, round, assistant)
				if err != nil {
					terminalReason = "max_round_fallback_failed"
					return l.fail(ctx, sink, round, err)
				}
				maxRoundFallback = true
			case toolDispatchDisabledMaxCalls:
				assistant, err = finalizeMaxToolCallResponse(ctx, sink, round, assistant)
				if err != nil {
					terminalReason = "max_tool_calls_fallback_failed"
					return l.fail(ctx, sink, round, err)
				}
				maxToolCallsFallback = true
				terminalReason = "max_tool_calls_fallback"
			case toolDispatchDisabledNoProgress:
				assistant, err = finalizeNoProgressToolCallResponse(ctx, sink, round, assistant)
				if err != nil {
					terminalReason = "no_progress_fallback_failed"
					return l.fail(ctx, sink, round, err)
				}
				noProgressFallback = true
				terminalReason = "no_progress_fallback"
			case toolDispatchDisabledNoTools:
				err := fmt.Errorf("%w: model requested %d tool calls while no tools were available", ErrToolDispatchUnavailable, len(assistant.ToolCalls))
				terminalReason = "tool_dispatch_unavailable"
				return l.fail(ctx, sink, round, err)
			case toolDispatchDisabledModelTools:
				err := fmt.Errorf("%w: model %q requested tool calls despite declaring tools unsupported", ErrUnsupportedModelCapability, l.languageModel.ModelID())
				terminalReason = "model_tools_unsupported"
				return l.fail(ctx, sink, round, err)
			}
		} else if len(assistant.ToolCalls) > 0 && len(executions) == 0 {
			err := fmt.Errorf("%w: model requested %d tool calls after dispatch completed", ErrUnresolvedToolCall, len(assistant.ToolCalls))
			terminalReason = "unresolved_tool_call"
			return l.fail(ctx, sink, round, err)
		}
		toolCallsUsed += len(executions)
		if stalled, observeErr := progress.observeRound(executions); observeErr != nil {
			terminalReason = "progress_guard_failed"
			return l.fail(ctx, sink, round, observeErr)
		} else if stalled {
			forceNoProgressSynthesis = true
			l.observeProtection(ctx, toolcall.ProtectionEvent{Kind: toolcall.ProtectionLoopDetected, Time: time.Now(), Round: round, Reason: "semantic_no_progress"})
			l.observeProtection(ctx, toolcall.ProtectionEvent{Kind: toolcall.ProtectionNoProgressSynthesis, Time: time.Now(), Round: round + 1, Reason: "semantic_no_progress"})
			slog.DebugContext(ctx, "turn semantic tool loop detected",
				"round", round,
				"tool_calls", len(executions),
			)
		}
		history = append(history, assistant)
		turnMessages = append(turnMessages, assistant)
		if len(executions) == 0 || isMaxRound {
			terminalReason = "completed"
			if maxRoundFallback {
				terminalReason = "max_rounds_fallback"
			} else if maxToolCallsFallback {
				terminalReason = "max_tool_calls_fallback"
			} else if noProgressFallback {
				terminalReason = "no_progress_fallback"
			}
			slog.DebugContext(ctx, "turn completed",
				"round", round,
				"assistant_bytes", len(assistant.Content),
				"tool_calls", len(executions),
				"tool_calls_used", toolCallsUsed,
				"max_round", isMaxRound,
			)
			result := Result{
				Message:  assistant,
				Rounds:   round,
				Messages: model.CloneMessages(turnMessages),
			}
			if err := emit(ctx, sink, Event{
				Kind:    EventCompleted,
				Round:   round,
				Message: assistant,
			}); err != nil {
				terminalReason = "completion_sink_failed"
				return Result{}, err
			}
			return result, nil
		}

		for _, execution := range executions {
			toolResult := execution.result
			content, err := json.Marshal(toolResult)
			if err != nil {
				terminalReason = "tool_result_encoding_failed"
				return l.fail(
					ctx,
					sink,
					round,
					fmt.Errorf("encode tool result %q: %w", execution.call.Name, err),
				)
			}
			toolMessage := model.Message{
				Role:              model.RoleTool,
				Content:           string(content),
				ToolCallID:        execution.call.ID,
				ToolName:          execution.call.Name,
				ToolResultIsError: execution.err != nil || toolResult.Denied || toolResult.Failure != nil,
			}
			history = append(history, toolMessage)
			turnMessages = append(turnMessages, toolMessage)
		}
	}
}

func finalizeMaxRoundToolCallResponse(
	ctx context.Context,
	sink Sink,
	round int,
	assistant model.Message,
) (model.Message, error) {
	slog.DebugContext(ctx, "turn ignored tool calls after max rounds",
		"round", round,
	)
	return finalizeDisabledToolCallResponse(ctx, sink, round, assistant, MaxRoundsFallback, "max-round")
}

func finalizeMaxToolCallResponse(
	ctx context.Context,
	sink Sink,
	round int,
	assistant model.Message,
) (model.Message, error) {
	slog.DebugContext(ctx, "turn ignored tool calls after max tool calls",
		"round", round,
	)
	return finalizeDisabledToolCallResponse(ctx, sink, round, assistant, MaxToolCallsFallback, "max-tool-calls")
}

func finalizeNoProgressToolCallResponse(
	ctx context.Context,
	sink Sink,
	round int,
	assistant model.Message,
) (model.Message, error) {
	slog.DebugContext(ctx, "turn ignored tool calls after no-progress detection",
		"round", round,
	)
	return finalizeDisabledToolCallResponse(ctx, sink, round, assistant, NoProgressFallback, "no-progress")
}

func finalizeDisabledToolCallResponse(
	ctx context.Context,
	sink Sink,
	round int,
	assistant model.Message,
	addition string,
	label string,
) (model.Message, error) {
	ignoredToolCalls := len(assistant.ToolCalls)
	assistant.ToolCalls = nil
	if strings.TrimSpace(assistant.Content) != "" {
		addition = "\n\n" + addition
	}
	assistant.Content += addition
	slog.DebugContext(ctx, "turn appended disabled-tool fallback",
		"round", round,
		"ignored_tool_calls", ignoredToolCalls,
		"fallback", label,
	)
	if err := emit(ctx, sink, Event{
		Kind:  EventTextDelta,
		Round: round,
		Text:  addition,
	}); err != nil {
		return model.Message{}, fmt.Errorf("emit %s fallback: %w", label, err)
	}
	return assistant, nil
}

type executedCall struct {
	call                tool.Call
	result              tool.Result
	err                 error
	suppressed          bool
	suppressionReason   string
	semanticFingerprint string
	repeatCount         int
	retryable           bool
}

func (l *Loop) runRound(
	parent context.Context,
	round int,
	request sdk.Request,
	dispatch toolDispatchState,
	progress *progressGuard,
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
	if dispatch.remainingToolCalls > 0 && len(requestedCalls) > dispatch.remainingToolCalls {
		slog.DebugContext(parent, "turn tool call budget exceeded",
			"round", round,
			"requested_tool_calls", len(requestedCalls),
			"remaining_tool_calls", dispatch.remainingToolCalls,
		)
		return roundOutcome{
			assistant:  assistant,
			executions: []executedCall{},
			dispatch: toolDispatchState{
				reason: toolDispatchDisabledMaxCalls,
			},
		}, nil
	}
	if len(request.Tools) == 0 {
		return roundOutcome{}, fmt.Errorf("%w: dispatch enabled without published tools", ErrInvalidLoop)
	}

	calls := make([]tool.Call, 0, len(requestedCalls))
	for _, requestedCall := range requestedCalls {
		call, err := tool.NewCall(
			requestedCall.ID,
			requestedCall.Name,
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
			l.observeSuppression(roundContext, round, *suppressed)
			continue
		}
		pendingCalls = append(pendingCalls, call)
		pendingIndexes = append(pendingIndexes, index)
	}
	if len(pendingCalls) > 0 {
		dispatched := l.executeCalls(roundContext, pendingCalls)
		for index, execution := range dispatched {
			executions[pendingIndexes[index]] = execution
		}
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

func (l *Loop) newTurnContext(parent context.Context) (context.Context, context.CancelFunc) {
	if l.turnTimeout > 0 {
		return context.WithTimeout(parent, l.turnTimeout)
	}
	return context.WithCancel(parent)
}

func (l *Loop) newRoundContext(parent context.Context) (context.Context, context.CancelFunc) {
	if l.roundTimeout > 0 {
		return context.WithTimeout(parent, l.roundTimeout)
	}
	return context.WithCancel(parent)
}

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

func (l *Loop) streamRound(
	ctx context.Context,
	round int,
	request sdk.Request,
	sink Sink,
) (model.Message, []model.ToolCall, error) {
	startedAt := time.Now()
	slog.DebugContext(ctx, "model round stream opening",
		"round", round,
		"message_count", len(request.Messages),
		"tool_count", len(request.Tools),
	)
	stream, err := l.languageModel.Stream(ctx, request)
	if err != nil {
		slog.DebugContext(ctx, "model round stream open failed",
			"round", round,
			"error_type", fmt.Sprintf("%T", err),
		)
		return model.Message{}, nil, fmt.Errorf("stream model round %d: %w", round, err)
	}
	if stream == nil {
		return model.Message{}, nil, fmt.Errorf("stream model round %d: nil stream", round)
	}
	defer func() {
		closeErr := stream.Close()
		slog.DebugContext(ctx, "model round stream closed",
			"round", round,
			"duration_ms", time.Since(startedAt).Milliseconds(),
			"close_error", closeErr != nil,
		)
	}()
	assistant, calls, streamErr := consumeSDKStream(ctx, round, stream, sink)
	if streamErr != nil {
		return model.Message{}, nil, streamErr
	}
	slog.DebugContext(ctx, "model round stream consumed",
		"round", round,
		"assistant_bytes", len(assistant.Content),
		"tool_calls", len(calls),
	)
	return assistant, calls, nil
}

func consumeSDKStream(ctx context.Context, round int, stream sdk.Stream, sink Sink) (model.Message, []model.ToolCall, error) {
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
		case sdk.EventTextDelta:
			text.WriteString(event.Text)
			if err := emit(ctx, sink, Event{Kind: EventTextDelta, Round: round, Text: event.Text}); err != nil {
				return model.Message{}, nil, err
			}
		case sdk.EventToolCall:
			call := event.ToolCall
			call.Arguments = append(json.RawMessage(nil), call.Arguments...)
			calls = append(calls, call)
		case sdk.EventFinish:
			if text.Len() == 0 && len(calls) == 0 {
				return model.Message{}, nil, fmt.Errorf("model stream round %d: %w", round, ErrEmptyResponse)
			}
			return model.Message{Role: model.RoleAssistant, Content: text.String(), ToolCalls: calls}, calls, nil
		}
	}
	return model.Message{}, nil, fmt.Errorf("read model stream round %d: %w", round, sdk.ErrIncompleteStream)
}
func (l *Loop) observeSuppression(ctx context.Context, round int, execution executedCall) {
	event := toolcall.ProtectionEvent{
		Kind: toolcall.ProtectionCallSuppressed, Time: time.Now(), Round: round,
		ToolName: execution.call.Name, Reason: execution.suppressionReason,
		Fingerprint: execution.semanticFingerprint, RepeatCount: execution.repeatCount,
		Retryable: execution.retryable,
	}
	if execution.result.Failure != nil {
		event.ErrorCode = execution.result.Failure.Code
	}
	for _, definition := range l.tools.Definitions() {
		if definition.Name == execution.call.Name {
			event.ToolKind = permission.ToolKind(definition.Kind)
			break
		}
	}
	l.observeProtection(ctx, event)
	specific := event
	switch execution.suppressionReason {
	case "permission_retry":
		specific.Kind = toolcall.ProtectionPermissionSuppressed
	case "retry_budget_exhausted":
		specific.Kind = toolcall.ProtectionRetryBudgetExhausted
	default:
		return
	}
	l.observeProtection(ctx, specific)
}

func (l *Loop) observeProtection(ctx context.Context, event toolcall.ProtectionEvent) {
	if l == nil || l.tools == nil {
		return
	}
	l.tools.ObserveProtection(ctx, event)
}

func (l *Loop) fail(ctx context.Context, sink Sink, round int, err error) (Result, error) {
	if errors.Is(err, context.DeadlineExceeded) && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		observeCtx, observeCancel := contextutil.DetachedTimeout(ctx, protectionObserverTimeout)
		l.observeProtection(observeCtx, toolcall.ProtectionEvent{Kind: toolcall.ProtectionTurnDeadlineExceeded, Time: time.Now(), Round: round, Reason: "turn_deadline"})
		observeCancel()
	}
	slog.DebugContext(ctx, "turn failed",
		"round", round,
		"error_type", fmt.Sprintf("%T", err),
		"context_error", ctx.Err() != nil,
	)
	emitContext := ctx
	emitCancel := func() {}
	if ctx.Err() != nil {
		// A terminal failure still needs to reach adapters after cancellation,
		// but detached cleanup must remain bounded.
		emitContext, emitCancel = contextutil.DetachedTimeout(ctx, terminalEmitTimeout)
	}
	defer emitCancel()
	if emitErr := emit(emitContext, sink, Event{
		Kind:  EventFailed,
		Round: round,
		Err:   err,
	}); emitErr != nil {
		slog.DebugContext(emitContext, "turn failure event failed",
			"round", round,
			"error_type", fmt.Sprintf("%T", emitErr),
		)
		return Result{}, emitErr
	}
	return Result{}, err
}

func emit(ctx context.Context, sink Sink, event Event) error {
	if err := ctx.Err(); err != nil {
		slog.DebugContext(ctx, "turn event emission cancelled",
			"event_kind", event.Kind,
			"round", event.Round,
			"error_type", fmt.Sprintf("%T", err),
		)
		return fmt.Errorf("emit model/tool event: %w", err)
	}
	if err := sink(ctx, event); err != nil {
		slog.DebugContext(ctx, "turn event sink failed",
			"event_kind", event.Kind,
			"round", event.Round,
			"error_type", fmt.Sprintf("%T", err),
		)
		return fmt.Errorf("emit model/tool event: %w", err)
	}
	return nil
}
