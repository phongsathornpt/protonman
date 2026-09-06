package modelprofile

import sdk "github.com/projectTHORN/proton/proton-sdk"

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
