package agent

import "testing"

func TestCloneResultNormalizesLegacySummaryAndDetachesFindings(t *testing.T) {
	original := Result{
		AgentID: "agility-legacy",
		Summary: "legacy finding",
		Findings: []Finding{{
			Claim:      "claim",
			Confidence: "high",
			Evidence:   []EvidenceRef{{Tool: "read", Target: "session.go"}},
		}},
		Blockers: []string{"blocker"},
	}
	cloned := cloneResult(original)
	if cloned.Conclusion != "legacy finding" || cloned.Summary != "legacy finding" {
		t.Fatalf("normalized result=%#v", cloned)
	}
	cloned.Findings[0].Evidence[0].Target = "mutated.go"
	cloned.Blockers[0] = "changed"
	if original.Findings[0].Evidence[0].Target != "session.go" || original.Blockers[0] != "blocker" {
		t.Fatalf("clone aliased original: original=%#v clone=%#v", original, cloned)
	}
}

func TestCloneResultBackfillsSummaryFromConclusion(t *testing.T) {
	cloned := cloneResult(Result{Conclusion: "canonical conclusion"})
	if cloned.Summary != "canonical conclusion" {
		t.Fatalf("summary=%q", cloned.Summary)
	}
}
