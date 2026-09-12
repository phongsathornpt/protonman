package turn

import (
	"fmt"
	"time"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/core/workspace"
	"github.com/phongsathornpt/protonman/internal/engine/prompt"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Option configures a Loop during construction.
type Option func(*Loop) error

// WithSystemPromptSpec overrides the default system prompt composition.
// The prompt is composed dynamically using the configured workspace and
// actual request environment on every round.
func WithSystemPromptSpec(spec prompt.Spec) Option {
	return func(loop *Loop) error {
		clone := spec
		clone.ModelPromptHints = append([]string(nil), spec.ModelPromptHints...)
		clone.AvailableTools = append([]string(nil), spec.AvailableTools...)
		clone.ExtraInstructions = append([]string(nil), spec.ExtraInstructions...)
		loop.promptSpec = &clone
		return nil
	}
}

// WithWorkspacePolicy supplies the workspace safety boundary used for
// policy-checked reads that the turn performs itself, such as loading
// project instructions. The loop never mutates through this policy; tool
// execution keeps its own mutation gate in the tool-call service.
func WithWorkspacePolicy(policy *workspace.Workspace) Option {
	return func(loop *Loop) error {
		loop.workspacePolicy = policy
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

// WithMaxIdenticalNoProgressResults bounds the number of consecutive identical
// read-only results before the turn stops.
func WithMaxIdenticalNoProgressResults(limit int) Option {
	return func(loop *Loop) error {
		if limit < 0 {
			return fmt.Errorf("%w: max identical no-progress results cannot be negative", ErrInvalidLoop)
		}
		loop.maxIdenticalNoProgressResults = limit
		return nil
	}
}

// WithTurnTimeout bounds the cumulative execution duration of one turn.
// Zero disables this turn-level bound.
func WithTurnTimeout(timeout time.Duration) Option {
	return func(loop *Loop) error {
		if timeout < 0 {
			return fmt.Errorf("%w: turn timeout cannot be negative", ErrInvalidLoop)
		}
		loop.turnTimeout = timeout
		return nil
	}
}

// WithRoundTimeout bounds one model/tool round when callers do not provide a
// stricter timeout. Zero disables this round-level bound.
func WithRoundTimeout(timeout time.Duration) Option {
	return func(loop *Loop) error {
		if timeout < 0 {
			return fmt.Errorf("%w: round timeout cannot be negative", ErrInvalidLoop)
		}
		loop.roundTimeout = timeout
		return nil
	}
}

// WithToolTimeout bounds one tool call when callers do not provide a stricter
// context. Zero keeps the service default.
func WithToolTimeout(timeout time.Duration) Option {
	return func(loop *Loop) error {
		if timeout < 0 {
			return fmt.Errorf("%w: tool timeout cannot be negative", ErrInvalidLoop)
		}
		loop.toolTimeout = timeout
		return nil
	}
}

// WithMaxToolResultBytesPerRound bounds total tool result bytes retained in
// history during a single turn round. Zero disables the bound.
func WithMaxToolResultBytesPerRound(bytes int) Option {
	return func(loop *Loop) error {
		if bytes < 0 {
			return fmt.Errorf("%w: max tool result bytes per round cannot be negative", ErrInvalidLoop)
		}
		loop.maxToolResultBytesPerRound = bytes
		return nil
	}
}

// WithMaxToolResultBytesPerTurn bounds total tool result bytes retained in
// history across all rounds in one turn. Zero disables the bound.
func WithMaxToolResultBytesPerTurn(bytes int) Option {
	return func(loop *Loop) error {
		if bytes < 0 {
			return fmt.Errorf("%w: max tool result bytes per turn cannot be negative", ErrInvalidLoop)
		}
		loop.maxToolResultBytesPerTurn = bytes
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

// WithRuntimeContext configures asynchronous external turn context providers.
func WithRuntimeContext(provider RuntimeContextProvider) Option {
	return func(loop *Loop) error {
		loop.runtimeContext = provider
		return nil
	}
}
