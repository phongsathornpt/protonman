// Package turn coordinates one model response stream with permission-aware
// tool execution.
package turn

import (
	"context"
	"errors"
	"fmt"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"log/slog"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

const (
	terminalEmitTimeout       = runtimepolicy.TerminalEmitTimeout
	protectionObserverTimeout = runtimepolicy.ProtectionObserverTimeout

	// DefaultMaxToolCalls is the default cumulative maximum number of tool
	// calls per turn.
	DefaultMaxToolCalls = runtimepolicy.TurnMaxToolCalls
	// DefaultTurnTimeout bounds one complete model/tool turn.
	DefaultTurnTimeout = runtimepolicy.TurnTimeout
	// DefaultRoundTimeout bounds a turn round when callers do not provide a
	// stricter timeout.
	DefaultRoundTimeout               = runtimepolicy.RoundTimeout
	DefaultMaxToolResultBytesPerRound = runtimepolicy.TurnToolResultBytesPerRound
	DefaultMaxToolResultBytesPerTurn  = runtimepolicy.TurnToolResultBytesPerTurn
	defaultMaxToolCalls               = DefaultMaxToolCalls
	defaultMaxParallelRead            = 4
	skillPromptMarker                 = "<!-- proton:skill-catalog -->"
)

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

// SoftToolBudgetPrompt nudges the model toward completion before the hard tool-call budget is exhausted.
const SoftToolBudgetPrompt = `TOOL BUDGET NOTICE

A substantial portion of this turn's tool-call budget has been used. Reassess whether the requested outcome is already complete. Prioritize only required work and verification, avoid optional exploration, and finish as soon as the task is complete.`

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
	// ErrEmptyResponse indicates that the provider completed without text or tool calls.
	ErrEmptyResponse = errors.New("model returned an empty response")
	// ErrDuplicateToolCall indicates that one model response reused a call ID.
	ErrDuplicateToolCall = errors.New("duplicate model tool call")
	// ErrToolDispatchUnavailable indicates that a model requested tools when no
	// tools were available for the current round.
	ErrToolDispatchUnavailable = errors.New("tool dispatch unavailable")
	// ErrUnresolvedToolCall indicates that a requested call had no execution result.
	ErrUnresolvedToolCall = errors.New("unresolved model tool call")
	// ErrGroundingUnavailable indicates that required empirical evidence cannot be obtained with the available tools.
	ErrGroundingUnavailable = errors.New("grounding unavailable")
	// ErrUnsupportedModelCapability indicates that the active model cannot satisfy a turn requirement.
	ErrUnsupportedModelCapability = errors.New("unsupported model capability")
	// ErrContextBudgetExceeded indicates that the request cannot fit the active model context safely.
	ErrContextBudgetExceeded = errors.New("model context budget exceeded")
)

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
	// EventRetryScheduled reports a bounded model/provider retry before its wait begins.
	EventRetryScheduled EventKind = "retry_scheduled"
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
	Retry   sdk.RetryEvent
	Message model.Message
	Err     error
}

// Sink receives loop events in emission order.
type Sink func(context.Context, Event) error

// Runner is the injectable model/tool loop used by TUI, headless, and ACP adapters.
type Runner interface {
	Run(context.Context, []model.Message, Sink) (Result, error)
}

// Result contains the latest replay-safe turn checkpoint. On success, Message is the final assistant response. On failure, Message may be empty while Messages, Rounds, and Verification describe only fully committed rounds.
type Result struct {
	Message      model.Message
	Rounds       int
	Verification VerificationState
	// ReplaySafe is true when Messages is a complete checkpoint that may be
	// persisted even if Run returns an error.
	ReplaySafe bool
	// Messages contains the assistant/tool messages produced during this run.
	// Ownership transfers to the caller on return; the runner must not mutate
	// this slice or its nested payloads afterward. It excludes caller-supplied
	// history and generated system prompt material.
	Messages []model.Message
}

type Loop struct {
	languageModel                 sdk.LanguageModel
	tools                         *toolcall.Service
	maxToolCalls                  int
	maxIdenticalNoProgressResults int
	turnTimeout                   time.Duration
	roundTimeout                  time.Duration
	toolTimeout                   time.Duration
	maxParallelReads              int
	maxToolResultBytesPerRound    int
	maxToolResultBytesPerTurn     int
	groundingEvidence             tool.EvidenceKind
	reasoningEffort               sdk.ReasoningEffort
	reasoningExplicit             bool
	promptSpec                    *prompt.Spec
	workspacePolicy               *workspace.Workspace
	skills                        []skill.CatalogItem
	skillRegistry                 *skill.Registry
	runtimeContext                RuntimeContextProvider
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
		maxToolCalls:                  defaultMaxToolCalls,
		maxIdenticalNoProgressResults: defaultMaxIdenticalNoProgressResults,
		turnTimeout:                   DefaultTurnTimeout,
		roundTimeout:                  DefaultRoundTimeout,
		maxParallelReads:              defaultMaxParallelRead,
		maxToolResultBytesPerRound:    DefaultMaxToolResultBytesPerRound,
		maxToolResultBytesPerTurn:     DefaultMaxToolResultBytesPerTurn,
	}
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(loop); err != nil {
			return nil, err
		}
	}
	if loop.maxToolCalls == 0 && loop.turnTimeout == 0 {
		return nil, fmt.Errorf("%w: at least one of max tool calls or turn timeout must be bounded", ErrInvalidLoop)
	}
	return loop, nil
}

// ReasoningPolicy returns the configured reasoning preference and whether it is an explicit override.
// CloneWithActiveGoal creates an independent loop with the same model and
// execution policy while replacing only the managed prompt goal.
func (l *Loop) CloneWithActiveGoal(goal string) (*Loop, error) {
	if l == nil {
		return nil, fmt.Errorf("%w: loop is required", ErrInvalidLoop)
	}
	clone, err := l.CloneWithTools(l.tools)
	if err != nil {
		return nil, err
	}
	if clone.promptSpec == nil {
		return nil, fmt.Errorf("%w: conversation does not use a managed prompt", ErrInvalidLoop)
	}
	clone.promptSpec.ActiveGoal = strings.TrimSpace(goal)
	return clone, nil
}

func (l *Loop) ReasoningPolicy() (sdk.ReasoningEffort, bool) {
	if l == nil {
		return sdk.ReasoningDefault, false
	}
	return l.reasoningEffort, l.reasoningExplicit
}

// CloneWithReasoningEffort creates an independent loop with a session-local reasoning policy.
func (l *Loop) CloneWithReasoningEffort(effort sdk.ReasoningEffort, explicit bool) (*Loop, error) {
	if l == nil {
		return nil, fmt.Errorf("%w: loop is required", ErrInvalidLoop)
	}
	if !effort.Valid() {
		return nil, fmt.Errorf("%w: unsupported reasoning effort %q", ErrInvalidLoop, effort)
	}
	clone, err := l.CloneWithTools(l.tools)
	if err != nil {
		return nil, err
	}
	clone.reasoningEffort = effort
	clone.reasoningExplicit = explicit && effort != sdk.ReasoningDefault
	if _, err := clone.resolveReasoningPolicy(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedModelCapability, err)
	}
	return clone, nil
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
	if l.promptSpec != nil {
		spec := *l.promptSpec
		spec.ModelPromptHints = append([]string(nil), l.promptSpec.ModelPromptHints...)
		spec.AvailableTools = append([]string(nil), l.promptSpec.AvailableTools...)
		spec.ExtraInstructions = append([]string(nil), l.promptSpec.ExtraInstructions...)
		clone.promptSpec = &spec
	}
	clone.skills = append([]skill.CatalogItem(nil), l.skills...)
	return &clone, nil
}

// Run executes model responses until one has no tool calls or another runtime safety bound is reached.
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
	ctx = workspace.WithMutationSession(turnContext)
	if l.runtimeContext != nil {
		defer l.runtimeContext.Finalize(ctx)
	}
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
		"max_tool_calls", l.maxToolCalls,
	)

	caps := l.languageModel.Capabilities()
	if !caps.Vision && messagesContainImages(messages) {
		terminalReason = "vision_unsupported"
		return l.fail(ctx, sink, 0, fmt.Errorf("%w: model %q does not support vision input", ErrUnsupportedModelCapability, l.languageModel.ModelID()))
	}

	history, promptExtras, projectInstructions := l.prepareTurnInput(ctx, messages)
	turnMessages := make([]model.Message, 0, 4)
	committedRounds := 0
	committedVerification := VerificationState{}
	checkpoint := func() Result {
		return Result{Rounds: committedRounds, Verification: committedVerification, Messages: turnMessages, ReplaySafe: len(turnMessages) > 0}
	}
	toolCallsUsed := 0
	softToolBudgetWarned := false
	definitions := l.tools.Definitions()
	progress := newProgressGuard(definitions, l.maxIdenticalNoProgressResults)
	resultBudget := newToolResultBudget(l.maxToolResultBytesPerRound, l.maxToolResultBytesPerTurn)
	verification := VerificationState{}
	forceNoProgressSynthesis := false
	grounding := newGroundingState(l.groundingEvidence)
	if grounding.pending() {
		if !caps.Tools {
			terminalReason = "grounding_model_tools_unsupported"
			return l.fail(ctx, sink, 0, fmt.Errorf("%w: model %q cannot satisfy required %s grounding without tool support", ErrUnsupportedModelCapability, l.languageModel.ModelID(), grounding.evidence))
		}
		if len(grounding.filterDefinitions(definitions)) == 0 {
			terminalReason = "grounding_tools_unavailable"
			return l.fail(ctx, sink, 0, fmt.Errorf("%w: no tools provide required %s evidence", ErrGroundingUnavailable, grounding.evidence))
		}
	}
	reasoningResolution, reasoningErr := l.resolveReasoningPolicy()
	if reasoningErr != nil {
		terminalReason = "reasoning_effort_unsupported"
		return l.fail(ctx, sink, 0, fmt.Errorf("%w: %v", ErrUnsupportedModelCapability, reasoningErr))
	}
	resolvedModel := resolvedModelStateFor(l.languageModel)
	modelProfileName := ""
	modelProfileMatch := ""
	modelCatalogOverride := false
	modelMetadataProvenance := ""
	if resolvedModel.has {
		modelProfileName = resolvedModel.profile.ProfileName
		modelProfileMatch = string(resolvedModel.profile.ProfileMatch)
		modelCatalogOverride = resolvedModel.profile.CatalogOverride
		modelMetadataProvenance = resolvedModel.profile.Provenance.Summary()
	}

	slog.DebugContext(ctx, "turn reasoning policy resolved",
		"requested", reasoningRequestedLabel(reasoningResolution),
		"effective", reasoningEffectiveLabel(reasoningResolution),
		"source", reasoningResolution.Source,
		"clamped", reasoningResolution.Clamped,
		"model_profile", modelProfileName,
		"model_profile_match", modelProfileMatch,
		"model_catalog_override", modelCatalogOverride,
		"model_metadata_provenance", modelMetadataProvenance,
	)

	for round := 1; ; round++ {
		roundsCompleted = round
		if l.runtimeContext != nil {
			runtimeMessages, runtimeErr := l.runtimeContext.Drain(ctx)
			if runtimeErr != nil {
				terminalReason = "runtime_context_failed"
				return l.failWithResult(ctx, sink, round, checkpoint(), fmt.Errorf("drain runtime context: %w", runtimeErr))
			}
			if len(runtimeMessages) > 0 {
				history = append(history, model.EnsureMessageIDs(runtimeMessages)...)
				slog.DebugContext(ctx, "turn runtime context injected", "round", round, "message_count", len(runtimeMessages))
			}
		}
		slog.DebugContext(ctx, "turn round started",
			"round", round,
			"history_messages", len(history),
		)

		request, dispatch, warned, err := l.prepareRoundRequest(
			ctx, history, definitions, promptExtras, projectInstructions,
			reasoningResolution, grounding, caps, toolCallsUsed,
			forceNoProgressSynthesis, softToolBudgetWarned, resolvedModel,
		)
		softToolBudgetWarned = warned
		if err != nil {
			if errors.Is(err, ErrContextBudgetExceeded) {
				terminalReason = "context_budget_exceeded"
			} else {
				terminalReason = "request_validation_failed"
			}
			return l.failWithResult(ctx, sink, round, checkpoint(), err)
		}

		roundSink := sink
		var bufferedEvents *runtimeEventBuffer
		if l.runtimeContext != nil && l.runtimeContext.Active(ctx) {
			bufferedEvents = &runtimeEventBuffer{}
			roundSink = bufferedEvents.sink
		}
		outcome, err := l.runRound(ctx, round, request, dispatch, progress, resultBudget, roundSink)
		if err != nil {
			if flushErr := bufferedEvents.flush(ctx, sink); flushErr != nil {
				terminalReason = "runtime_event_flush_failed"
				return l.failWithResult(ctx, sink, round, checkpoint(), flushErr)
			}
			terminalReason = "round_failed"
			return l.failWithResult(ctx, sink, round, checkpoint(), err)
		}
		assistant := outcome.assistant
		executions := outcome.executions
		maxToolCallsFallback := false
		noProgressFallback := false
		if len(assistant.ToolCalls) > 0 && !outcome.dispatch.enabled() {
			switch outcome.dispatch.reason {
			case toolDispatchDisabledMaxCalls:
				assistant, err = finalizeMaxToolCallResponse(ctx, sink, round, assistant)
				if err != nil {
					terminalReason = "max_tool_calls_fallback_failed"
					return l.failWithResult(ctx, sink, round, checkpoint(), err)
				}
				maxToolCallsFallback = true
				terminalReason = "max_tool_calls_fallback"
			case toolDispatchDisabledNoProgress:
				assistant, err = finalizeNoProgressToolCallResponse(ctx, sink, round, assistant)
				if err != nil {
					terminalReason = "no_progress_fallback_failed"
					return l.failWithResult(ctx, sink, round, checkpoint(), err)
				}
				noProgressFallback = true
				terminalReason = "no_progress_fallback"
			case toolDispatchDisabledNoTools:
				err := fmt.Errorf("%w: model requested %d tool calls while no tools were available", ErrToolDispatchUnavailable, len(assistant.ToolCalls))
				terminalReason = "tool_dispatch_unavailable"
				return l.failWithResult(ctx, sink, round, checkpoint(), err)
			case toolDispatchDisabledModelTools:
				err := fmt.Errorf("%w: model %q requested tool calls despite declaring tools unsupported", ErrUnsupportedModelCapability, l.languageModel.ModelID())
				terminalReason = "model_tools_unsupported"
				return l.failWithResult(ctx, sink, round, checkpoint(), err)
			}
		} else if len(assistant.ToolCalls) > 0 && len(executions) == 0 {
			err := fmt.Errorf("%w: model requested %d tool calls after dispatch completed", ErrUnresolvedToolCall, len(assistant.ToolCalls))
			terminalReason = "unresolved_tool_call"
			return l.failWithResult(ctx, sink, round, checkpoint(), err)
		}
		toolCallsUsed += len(executions)
		if grounding.observe(executions, definitions) {
			slog.DebugContext(ctx, "turn workspace grounding satisfied", "round", round, "evidence", grounding.evidence)
		}
		verification.observe(executions, definitions)
		if stalled, observeErr := progress.observeRound(executions); observeErr != nil {
			terminalReason = "progress_guard_failed"
			return l.failWithResult(ctx, sink, round, checkpoint(), observeErr)
		} else if stalled {
			forceNoProgressSynthesis = !grounding.pending()
			l.observeProtection(ctx, toolcall.ProtectionEvent{Kind: toolcall.ProtectionLoopDetected, Time: time.Now(), Round: round, Reason: "semantic_no_progress"})
			l.observeProtection(ctx, toolcall.ProtectionEvent{Kind: toolcall.ProtectionNoProgressSynthesis, Time: time.Now(), Round: round + 1, Reason: "semantic_no_progress"})
			slog.DebugContext(ctx, "turn semantic tool loop detected",
				"round", round,
				"tool_calls", len(executions),
			)
		}
		if grounding.pending() && len(executions) == 0 && !maxToolCallsFallback && !noProgressFallback {
			if err := bufferedEvents.flush(ctx, sink); err != nil {
				terminalReason = "runtime_event_flush_failed"
				return l.failWithResult(ctx, sink, round, checkpoint(), err)
			}
			history = append(history, assistant)
			turnMessages = append(turnMessages, assistant)
			committedRounds = round
			committedVerification = verification
			if grounding.recordMiss() {
				terminalReason = "grounding_not_observed"
				return l.failWithResult(ctx, sink, round, checkpoint(), fmt.Errorf("%w: model %q did not gather required %s evidence", ErrGroundingUnavailable, l.languageModel.ModelID(), grounding.evidence))
			}
			slog.DebugContext(ctx, "turn final synthesis deferred for grounding", "round", round, "evidence", grounding.evidence)
			continue
		}
		if len(executions) == 0 {
			runtimeMessages, deferred, runtimeErr := l.completionRuntimeContext(ctx)
			if runtimeErr != nil {
				terminalReason = "runtime_context_failed"
				return l.failWithResult(ctx, sink, round, checkpoint(), fmt.Errorf("await runtime context: %w", runtimeErr))
			}
			if deferred {
				if len(runtimeMessages) > 0 {
					history = append(history, runtimeMessages...)
				}
				slog.DebugContext(ctx, "turn final synthesis deferred for runtime context", "round", round, "message_count", len(runtimeMessages))
				continue
			}
			if err := bufferedEvents.flush(ctx, sink); err != nil {
				terminalReason = "runtime_event_flush_failed"
				return l.failWithResult(ctx, sink, round, checkpoint(), err)
			}
			history = append(history, assistant)
			turnMessages = append(turnMessages, assistant)
			terminalReason = "completed"
			if maxToolCallsFallback {
				terminalReason = "max_tool_calls_fallback"
			} else if noProgressFallback {
				terminalReason = "no_progress_fallback"
			}
			slog.DebugContext(ctx, "turn completed",
				"round", round,
				"assistant_bytes", len(assistant.Content),
				"tool_calls", len(executions),
				"tool_calls_used", toolCallsUsed,
			)
			result := Result{
				Message:      assistant,
				Rounds:       round,
				Verification: verification,
				ReplaySafe:   true,
				Messages:     turnMessages,
			}
			if err := emit(ctx, sink, Event{
				Kind:    EventCompleted,
				Round:   round,
				Message: assistant,
			}); err != nil {
				terminalReason = "completion_sink_failed"
				return result, err
			}
			return result, nil
		}

		if err := bufferedEvents.flush(ctx, sink); err != nil {
			terminalReason = "runtime_event_flush_failed"
			return l.failWithResult(ctx, sink, round, checkpoint(), err)
		}
		toolMessages, err := toolMessagesForExecutions(executions)
		if err != nil {
			terminalReason = "tool_result_encoding_failed"
			return l.failWithResult(ctx, sink, round, checkpoint(), err)
		}
		history = append(history, assistant)
		history = append(history, toolMessages...)
		turnMessages = append(turnMessages, assistant)
		turnMessages = append(turnMessages, toolMessages...)
		committedRounds = round
		committedVerification = verification
	}
}
