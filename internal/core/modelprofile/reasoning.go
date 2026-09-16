package modelprofile

import (
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

// ReasoningSource identifies the policy layer that selected the effective effort.
type ReasoningSource string

const (
	ReasoningSourceProviderDefault ReasoningSource = "provider_default"
	ReasoningSourceAgentProfile    ReasoningSource = "agent_profile"
	ReasoningSourceExplicit        ReasoningSource = "explicit"
)

// ReasoningResolution records the requested and effective effort plus provenance.
type ReasoningResolution struct {
	Requested domain.ReasoningEffort
	Effective domain.ReasoningEffort
	Source    ReasoningSource
	Clamped   bool
}

// ResolveReasoning applies explicit or portable profile semantics in one place.
func (r Resolved) ResolveReasoning(requested domain.ReasoningEffort, explicit bool) (ReasoningResolution, error) {
	resolution := ReasoningResolution{Requested: requested, Source: ReasoningSourceProviderDefault}
	if requested == domain.ReasoningDefault {
		return resolution, nil
	}
	if explicit {
		effective, err := r.ResolveExplicitReasoning(requested)
		if err != nil {
			return resolution, err
		}
		resolution.Effective = effective
		resolution.Source = ReasoningSourceExplicit
		return resolution, nil
	}
	effective, ok := r.ResolveProfileReasoning(requested)
	if !ok {
		return resolution, nil
	}
	resolution.Effective = effective
	resolution.Source = ReasoningSourceAgentProfile
	resolution.Clamped = effective != requested
	return resolution, nil
}

// ResolveProfileReasoning maps a portable agent-profile preference onto the
// levels known to be supported by this model. Unknown/unsupported profiles
// preserve the provider default by returning ok=false.
func (r Resolved) ResolveProfileReasoning(requested domain.ReasoningEffort) (effective domain.ReasoningEffort, ok bool) {
	if requested == domain.ReasoningDefault || r.Reasoning.Support != SupportYes {
		return domain.ReasoningDefault, false
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
		return domain.ReasoningDefault, false
	}
	best := domain.ReasoningDefault
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
	if best == domain.ReasoningDefault {
		return domain.ReasoningDefault, false
	}
	return best, true
}

func reasoningRank(effort domain.ReasoningEffort) (int, bool) {
	switch effort {
	case domain.ReasoningNone:
		return 0, true
	case domain.ReasoningLow:
		return 1, true
	case domain.ReasoningMedium:
		return 2, true
	case domain.ReasoningHigh:
		return 3, true
	case domain.ReasoningXHigh:
		return 4, true
	case domain.ReasoningMax:
		return 5, true
	default:
		return 0, false
	}
}

// ResolveExplicitReasoning validates a user-selected effort. Unlike portable
// profile preferences, explicit selections are never silently clamped.
func (r Resolved) ResolveExplicitReasoning(requested domain.ReasoningEffort) (domain.ReasoningEffort, error) {
	if requested == domain.ReasoningDefault {
		return domain.ReasoningDefault, nil
	}
	if !requested.Valid() {
		return domain.ReasoningDefault, fmt.Errorf("invalid reasoning effort %q", requested)
	}
	switch r.Reasoning.Support {
	case SupportNo:
		return domain.ReasoningDefault, fmt.Errorf("model %q does not support reasoning effort", r.ModelID)
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
	return domain.ReasoningDefault, fmt.Errorf("model %q does not support reasoning effort %q; supported levels: %s", r.ModelID, requested, formatReasoningLevels(r.Reasoning.Levels))
}

func formatReasoningLevels(levels []domain.ReasoningEffort) string {
	parts := make([]string, 0, len(levels))
	for _, level := range levels {
		parts = append(parts, string(level))
	}
	return strings.Join(parts, ", ")
}
