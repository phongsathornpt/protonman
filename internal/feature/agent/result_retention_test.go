package agent

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

func TestCompactRetainedResultBoundsPayload(t *testing.T) {
	result := Result{
		AgentID:        "agent-1",
		Summary:        strings.Repeat("s", runtimepolicy.AgentRetainedResultBytes),
		Evidence:       []EvidenceRef{{Tool: "read", Target: strings.Repeat("e", 32*1024)}},
		ChangedTargets: []string{strings.Repeat("p", 32*1024)},
	}
	got := compactRetainedResult(result)
	payload := len(got.Summary)
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
