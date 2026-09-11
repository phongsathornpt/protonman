package agent

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

func TestCompactRetainedResultBoundsPayload(t *testing.T) {
	result := Result{
		AgentID: "agent-1", Conclusion: strings.Repeat("c", 48*1024), Summary: strings.Repeat("s", 48*1024),
		Findings: []Finding{{Claim: strings.Repeat("f", 24*1024), Confidence: "high", Evidence: []EvidenceRef{{Tool: "read", Target: "finding.go"}}}},
		Blockers: []string{strings.Repeat("b", 16*1024)}, Evidence: []EvidenceRef{{Tool: "read", Target: strings.Repeat("e", 32*1024)}},
		ChangedTargets: []string{strings.Repeat("p", 32*1024)},
	}
	got := compactRetainedResult(result)
	payload := len(got.Conclusion) + len(got.Summary)
	for _, finding := range got.Findings {
		payload += len(finding.Claim) + len(finding.Confidence)
		for _, evidence := range finding.Evidence {
			payload += len(evidence.Tool) + len(evidence.Target)
		}
	}
	for _, blocker := range got.Blockers {
		payload += len(blocker)
	}
	for _, evidence := range got.Evidence {
		payload += len(evidence.Tool) + len(evidence.Target)
	}
	for _, target := range got.ChangedTargets {
		payload += len(target)
	}
	if payload > runtimepolicy.AgentRetainedResultBytes {
		t.Fatalf("retained payload=%d, want <=%d", payload, runtimepolicy.AgentRetainedResultBytes)
	}
	if got.AgentID != result.AgentID {
		t.Fatalf("agent id=%q, want %q", got.AgentID, result.AgentID)
	}
}
