package conversation

import (
	"fmt"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

// CostEstimator measures model-facing cost for a candidate message slice.
// Unlike Retain's byte accounting this is intentionally transport-neutral and
// is used by model context compaction, not by live memory retention.
type CostEstimator func([]domain.Message) (int, error)

// RetainByCost drops the oldest protocol-safe message groups until estimator
// reports a value within budget. Leading system messages and the newest span
// are always retained, matching Retain's soft-limit semantics. Historical tool
// groups are compacted before model-cost evaluation using the same policy as
// normal retention.
func RetainByCost(messages []domain.Message, recentMessages, maxHistoricalToolResultBytes, budget int, estimator CostEstimator) ([]domain.Message, error) {
	if len(messages) == 0 {
		return nil, nil
	}
	if estimator == nil {
		return nil, fmt.Errorf("conversation cost estimator is required")
	}
	messages = compactHistoricalToolGroups(messages, recentMessages, maxHistoricalToolResultBytes)
	if budget <= 0 {
		return messages, nil
	}
	cost, err := estimator(messages)
	if err != nil {
		return nil, err
	}
	if cost <= budget {
		return messages, nil
	}

	leadingSystems := 0
	for leadingSystems < len(messages) && messages[leadingSystems].Role == domain.RoleSystem {
		leadingSystems++
	}
	spans := buildSpans(messages, leadingSystems)
	if len(spans) == 0 {
		return append([]domain.Message(nil), messages...), nil
	}

	// Retain at least the newest protocol span even when the protected minimum
	// itself exceeds budget. A context error can then report the real condition
	// instead of silently deleting the active turn.
	drop := 0
	for drop < len(spans)-1 && cost > budget {
		drop++
		candidate := costRetentionCandidate(messages, leadingSystems, spans[drop].start)
		cost, err = estimator(candidate)
		if err != nil {
			return nil, err
		}
	}
	return costRetentionCandidate(messages, leadingSystems, spans[drop].start), nil
}

func costRetentionCandidate(messages []domain.Message, leadingSystems, start int) []domain.Message {
	out := make([]domain.Message, 0, leadingSystems+len(messages)-start)
	out = append(out, messages[:leadingSystems]...)
	out = append(out, messages[start:]...)
	return out
}
