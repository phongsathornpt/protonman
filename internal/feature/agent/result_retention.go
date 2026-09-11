package agent

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

func compactRetainedResult(result Result) Result {
	err := result.Err
	result = cloneResult(result)
	result.Err = err
	budget := runtimepolicy.AgentRetainedResultBytes
	conclusion := result.Conclusion
	if strings.TrimSpace(conclusion) == "" {
		conclusion = result.Summary
	}
	result.Conclusion = retainText(conclusion, &budget)
	result.Findings = retainFindings(result.Findings, &budget)
	result.Blockers = retainStrings(result.Blockers, &budget)
	result.Evidence = retainEvidence(result.Evidence, &budget)
	result.ChangedTargets = retainStrings(result.ChangedTargets, &budget)
	if result.Summary == "" || result.Summary == conclusion {
		result.Summary = result.Conclusion
	} else {
		result.Summary = retainText(result.Summary, &budget)
	}
	return result
}

func retainText(value string, budget *int) string {
	if budget == nil || *budget <= 0 || value == "" {
		return ""
	}
	value = strings.ToValidUTF8(value, "")
	if len(value) > *budget {
		value = truncatePersistentText(value, *budget)
	}
	*budget -= len(value)
	return value
}

func retainFindings(values []Finding, budget *int) []Finding {
	if len(values) == 0 || budget == nil || *budget <= 0 {
		return nil
	}
	out := make([]Finding, 0, len(values))
	for _, value := range values {
		claim := retainText(value.Claim, budget)
		if claim == "" {
			break
		}
		confidence := retainText(value.Confidence, budget)
		evidence := retainEvidence(value.Evidence, budget)
		out = append(out, Finding{Claim: claim, Confidence: confidence, Evidence: evidence})
		if *budget <= 0 {
			break
		}
	}
	return out
}

func retainEvidence(values []EvidenceRef, budget *int) []EvidenceRef {
	if len(values) == 0 || budget == nil || *budget <= 0 {
		return nil
	}
	out := make([]EvidenceRef, 0, len(values))
	for _, value := range values {
		tool := retainText(value.Tool, budget)
		target := retainText(value.Target, budget)
		if tool == "" && target == "" {
			break
		}
		out = append(out, EvidenceRef{Tool: tool, Target: target})
		if *budget <= 0 {
			break
		}
	}
	return out
}

func retainStrings(values []string, budget *int) []string {
	if len(values) == 0 || budget == nil || *budget <= 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		retained := retainText(value, budget)
		if retained == "" {
			break
		}
		out = append(out, retained)
		if *budget <= 0 {
			break
		}
	}
	return out
}
