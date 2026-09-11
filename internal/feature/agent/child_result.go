package agent

import (
	"encoding/json"
	"strings"
)

const (
	childResultOpenTag        = "<proton-subagent-result>"
	childResultCloseTag       = "</proton-subagent-result>"
	maxStructuredFindings     = 32
	maxFindingClaimBytes      = 4096
	maxFindingEvidence        = 16
	maxStructuredBlockers     = 16
	maxStructuredBlockerBytes = 2048
)

type childResultEnvelope struct {
	Conclusion string    `json:"conclusion"`
	Findings   []Finding `json:"findings,omitempty"`
	Blockers   []string  `json:"blockers,omitempty"`
}

func parseChildSemanticResult(content string, observed []EvidenceRef) (string, []Finding, []string) {
	raw := strings.TrimSpace(strings.ToValidUTF8(content, "�"))
	fallback := raw
	start := strings.LastIndex(raw, childResultOpenTag)
	if start < 0 {
		return fallbackConclusion(fallback), nil, nil
	}
	rest := raw[start+len(childResultOpenTag):]
	end := strings.Index(rest, childResultCloseTag)
	if end < 0 {
		if prefix := strings.TrimSpace(raw[:start]); prefix != "" {
			fallback = prefix
		}
		return fallbackConclusion(fallback), nil, nil
	}

	var envelope childResultEnvelope
	if err := json.Unmarshal([]byte(strings.TrimSpace(rest[:end])), &envelope); err != nil {
		if prefix := strings.TrimSpace(raw[:start]); prefix != "" {
			fallback = prefix
		}
		return fallbackConclusion(fallback), nil, nil
	}

	conclusion := strings.TrimSpace(envelope.Conclusion)
	if conclusion == "" {
		conclusion = strings.TrimSpace(raw[:start])
	}
	conclusion = fallbackConclusion(conclusion)
	findings := normalizeStructuredFindings(envelope.Findings, observed)
	blockers := normalizeStructuredStrings(envelope.Blockers, maxStructuredBlockers, maxStructuredBlockerBytes)
	return conclusion, findings, blockers
}

func fallbackConclusion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "Task completed with no final text response."
	}
	return truncateSummary(value, maxSummaryBytes)
}
func normalizeStructuredFindings(values []Finding, observed []EvidenceRef) []Finding {
	if len(values) == 0 {
		return nil
	}
	allowed := make(map[string]EvidenceRef, len(observed))
	for _, ref := range observed {
		ref.Tool = strings.TrimSpace(ref.Tool)
		ref.Target = strings.TrimSpace(ref.Target)
		allowed[evidenceKey(ref)] = ref
	}
	out := make([]Finding, 0, min(len(values), maxStructuredFindings))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		claim := strings.TrimSpace(value.Claim)
		if claim == "" {
			continue
		}
		claim = truncatePersistentText(claim, maxFindingClaimBytes)
		if _, exists := seen[claim]; exists {
			continue
		}
		seen[claim] = struct{}{}
		value.Claim = claim
		value.Confidence = normalizeFindingConfidence(value.Confidence)
		value.Evidence = validatedFindingEvidence(value.Evidence, allowed)
		out = append(out, value)
		if len(out) >= maxStructuredFindings {
			break
		}
	}
	return out
}
func normalizeFindingConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func validatedFindingEvidence(values []EvidenceRef, allowed map[string]EvidenceRef) []EvidenceRef {
	if len(values) == 0 || len(allowed) == 0 {
		return nil
	}
	out := make([]EvidenceRef, 0, min(len(values), maxFindingEvidence))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.Tool = strings.TrimSpace(value.Tool)
		value.Target = strings.TrimSpace(value.Target)
		key := evidenceKey(value)
		ref, ok := allowed[key]
		if !ok {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref)
		if len(out) >= maxFindingEvidence {
			break
		}
	}
	return out
}
func normalizeStructuredStrings(values []string, maxItems, maxBytes int) []string {
	if len(values) == 0 || maxItems <= 0 || maxBytes <= 0 {
		return nil
	}
	out := make([]string, 0, min(len(values), maxItems))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		value = truncatePersistentText(value, maxBytes)
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= maxItems {
			break
		}
	}
	return out
}

func evidenceKey(ref EvidenceRef) string {
	return strings.TrimSpace(ref.Tool) + "\x00" + strings.TrimSpace(ref.Target)
}

func normalizeResultCompatibility(result Result) Result {
	if strings.TrimSpace(result.Conclusion) == "" {
		result.Conclusion = strings.TrimSpace(result.Summary)
	}
	if strings.TrimSpace(result.Summary) == "" {
		result.Summary = result.Conclusion
	}
	return result
}
