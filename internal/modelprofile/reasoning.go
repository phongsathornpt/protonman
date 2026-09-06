package modelprofile

import (
	"fmt"
	"strings"

	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// ResolveProfileReasoning maps a portable agent-profile preference onto the
// levels known to be supported by this model. Unknown/unsupported profiles
// preserve the provider default by returning ok=false.
func (r Resolved) ResolveProfileReasoning(requested sdk.ReasoningEffort) (effective sdk.ReasoningEffort, ok bool) {
	if requested == sdk.ReasoningDefault || r.Reasoning.Support != SupportYes {
		return sdk.ReasoningDefault, false
	}
	if len(r.Reasoning.Levels) == 0 {
		return requested, true
	}
	for _, level := range r.Reasoning.Levels {
		if level == requested {
			return requested, true
		}
	}
	requestedRank, ranked := reasoningRank(requested)
	if !ranked {
		return sdk.ReasoningDefault, false
	}
	best := sdk.ReasoningDefault
	bestDistance := int(^uint(0) >> 1)
	bestRank := -1
	for _, level := range r.Reasoning.Levels {
		rank, ok := reasoningRank(level)
		if !ok {
			continue
		}
		distance := rank - requestedRank
		if distance < 0 {
			distance = -distance
		}
		if distance < bestDistance || (distance == bestDistance && rank < bestRank) {
			best, bestDistance, bestRank = level, distance, rank
		}
	}
	if best == sdk.ReasoningDefault {
		return sdk.ReasoningDefault, false
	}
	return best, true
}

func reasoningRank(effort sdk.ReasoningEffort) (int, bool) {
	switch effort {
	case sdk.ReasoningNone:
		return 0, true
	case sdk.ReasoningLow:
		return 1, true
	case sdk.ReasoningMedium:
		return 2, true
	case sdk.ReasoningHigh:
		return 3, true
	case sdk.ReasoningXHigh:
		return 4, true
	case sdk.ReasoningMax:
		return 5, true
	default:
		return 0, false
	}
}

// ResolveExplicitReasoning validates a user-selected effort. Unlike portable
// profile preferences, explicit selections are never silently clamped.
func (r Resolved) ResolveExplicitReasoning(requested sdk.ReasoningEffort) (sdk.ReasoningEffort, error) {
	if requested == sdk.ReasoningDefault {
		return sdk.ReasoningDefault, nil
	}
	if !requested.Valid() {
		return sdk.ReasoningDefault, fmt.Errorf("invalid reasoning effort %q", requested)
	}
	switch r.Reasoning.Support {
	case SupportNo:
		return sdk.ReasoningDefault, fmt.Errorf("model %q does not support reasoning effort", r.ModelID)
	case SupportUnknown:
		return requested, nil
	}
	if len(r.Reasoning.Levels) == 0 {
		return requested, nil
	}
	for _, level := range r.Reasoning.Levels {
		if level == requested {
			return requested, nil
		}
	}
	return sdk.ReasoningDefault, fmt.Errorf("model %q does not support reasoning effort %q; supported levels: %s", r.ModelID, requested, formatReasoningLevels(r.Reasoning.Levels))
}

func formatReasoningLevels(levels []sdk.ReasoningEffort) string {
	parts := make([]string, 0, len(levels))
	for _, level := range levels {
		parts = append(parts, string(level))
	}
	return strings.Join(parts, ", ")
}
