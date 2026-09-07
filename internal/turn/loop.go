// Package turn coordinates one model response stream with permission-aware
// tool execution.
package turn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"log/slog"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/agentprompt"
	"github.com/projectTHORN/proton/internal/contextutil"
	"github.com/projectTHORN/proton/internal/model"
	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/workspace"
	sdk "github.com/projectTHORN/proton/proton-sdk"
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

type toolDispatchReason string

const (
	toolDispatchEnabled            toolDispatchReason = "enabled"
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
	Message      model.Message
	Rounds       int
	Verification VerificationState
	// Messages contains the assistant/tool messages produced during this run.
	// It excludes caller-supplied history and generated system prompt material.
	Messages []model.Message
}

// Option configures a Loop.
type Option func(*Loop) error

// WithSystemPromptSpec enables deterministic runtime prompt composition.
// The loop fills model, tool, skill, and execution-limit sections from the
// actual request environment on every round.
func WithSystemPromptSpec(spec agentprompt.Spec) Option {
	return func(loop *Loop) error {
		clone := spec
		clone.ToolNames = append([]string(nil), spec.ToolNames...)
		clone.ModelPromptHints = append([]string(nil), spec.ModelPromptHints...)
		clone.ExtraInstructions = append([]string(nil), spec.ExtraInstructions...)
		loop.promptSpec = &clone
		return nil
	}
}

// WithExplicitReasoningEffort sets a user-selected reasoning level. Known
// unsupported levels fail locally rather than being silently clamped.
func WithExplicitReasoningEffort(effort sdk.ReasoningEffort) Option {
	return func(loop *Loop) error {
		if !effort.Valid() {
			return fmt.Errorf("%w: unsupported reasoning effort %q", ErrInvalidLoop, effort)
		}
		loop.reasoningEffort = effort
		loop.reasoningExplicit = true
		return nil
	}
}

// WithReasoningEffort sets a portable reasoning preference. The loop only
// forwards it when the resolved model profile confirms reasoning support.
func WithReasoningEffort(effort sdk.ReasoningEffort) Option {
	return func(loop *Loop) error {
		if !effort.Valid() {
			return fmt.Errorf("%w: unsupported reasoning effort %q", ErrInvalidLoop, effort)
		}
		loop.reasoningEffort = effort
		return nil
	}
}

// WithGroundingEvidence requires one successful empirical observation of the
// requested evidence kind before broader tools or final synthesis are allowed.
func WithGroundingEvidence(evidence tool.EvidenceKind) Option {
	return func(loop *Loop) error {
		switch evidence {
		case tool.EvidenceNone, tool.EvidenceWorkspace, tool.EvidenceExternal:
			loop.groundingEvidence = evidence
			return nil
		default:
			return fmt.Errorf("%w: unsupported grounding evidence %q", ErrInvalidLoop, evidence)
		}
	}
}

// WithRequireInitialToolUse is retained for internal compatibility. Required
// initial tool use now means successful workspace grounding, not any tool call.
func WithRequireInitialToolUse(required bool) Option {
	if required {
		return WithGroundingEvidence(tool.EvidenceWorkspace)
	}
	return WithGroundingEvidence(tool.EvidenceNone)
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

func WithMaxToolResultBytesPerRound(limit int) Option {
	return func(loop *Loop) error {
		if limit < 0 {
			return fmt.Errorf("%w: max tool-result bytes per round cannot be negative", ErrInvalidLoop)
		}
		loop.maxToolResultBytesPerRound = limit
		return nil
	}
}

func WithMaxToolResultBytesPerTurn(limit int) Option {
	return func(loop *Loop) error {
		if limit < 0 {
			return fmt.Errorf("%w: max tool-result bytes per turn cannot be negative", ErrInvalidLoop)
		}
		loop.maxToolResultBytesPerTurn = limit
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
func shouldWarnSoftToolBudget(used, max int, warned bool) bool {
	if warned || max <= 0 || used <= 0 {
		return false
	}
	return used*5 >= max*3
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
	promptSpec                    *agentprompt.Spec
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
		spec.ToolNames = append([]string(nil), l.promptSpec.ToolNames...)
		spec.ModelPromptHints = append([]string(nil), l.promptSpec.ModelPromptHints...)
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
			return l.fail(ctx, sink, round, err)
		}

		outcome, err := l.runRound(ctx, round, request, dispatch, progress, resultBudget, sink)
		if err != nil {
			terminalReason = "round_failed"
			return l.fail(ctx, sink, round, err)
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
		if grounding.observe(executions, definitions) {
			slog.DebugContext(ctx, "turn workspace grounding satisfied", "round", round, "evidence", grounding.evidence)
		}
		verification.observe(executions, definitions)
		if stalled, observeErr := progress.observeRound(executions); observeErr != nil {
			terminalReason = "progress_guard_failed"
			return l.fail(ctx, sink, round, observeErr)
		} else if stalled {
			forceNoProgressSynthesis = !grounding.pending()
			l.observeProtection(ctx, toolcall.ProtectionEvent{Kind: toolcall.ProtectionLoopDetected, Time: time.Now(), Round: round, Reason: "semantic_no_progress"})
			l.observeProtection(ctx, toolcall.ProtectionEvent{Kind: toolcall.ProtectionNoProgressSynthesis, Time: time.Now(), Round: round + 1, Reason: "semantic_no_progress"})
			slog.DebugContext(ctx, "turn semantic tool loop detected",
				"round", round,
				"tool_calls", len(executions),
			)
		}
		history = append(history, assistant)
		turnMessages = append(turnMessages, assistant)
		if grounding.pending() && len(executions) == 0 && !maxToolCallsFallback && !noProgressFallback {
			if grounding.recordMiss() {
				terminalReason = "grounding_not_observed"
				return l.fail(ctx, sink, round, fmt.Errorf("%w: model %q did not gather required %s evidence", ErrGroundingUnavailable, l.languageModel.ModelID(), grounding.evidence))
			}
			slog.DebugContext(ctx, "turn final synthesis deferred for grounding", "round", round, "evidence", grounding.evidence)
			continue
		}
		if len(executions) == 0 {
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
				Messages:     model.CloneMessages(turnMessages),
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
