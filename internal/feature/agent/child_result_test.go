package agent

import (
	"strings"
	"testing"
)

func TestParseChildSemanticResultFallsBackToPlainText(t *testing.T) {
	conclusion, findings, blockers := parseChildSemanticResult("  found reconnect race  ", nil)
	if conclusion != "found reconnect race" {
		t.Fatalf("conclusion=%q", conclusion)
	}
	if len(findings) != 0 || len(blockers) != 0 {
		t.Fatalf("unexpected structured fields: findings=%#v blockers=%#v", findings, blockers)
	}
}

func TestParseChildSemanticResultValidatesFindingEvidence(t *testing.T) {
	content := `analysis prose
<proton-subagent-result>
{"conclusion":"listener is registered twice","findings":[{"claim":"reconnect duplicates the listener","confidence":"HIGH","evidence":[{"tool":"read","target":"session.go"},{"tool":"read","target":"invented.go"}]}],"blockers":["needs integration test"]}
</proton-subagent-result>`
	observed := []EvidenceRef{{Tool: "read", Target: "session.go"}}
	conclusion, findings, blockers := parseChildSemanticResult(content, observed)
	if conclusion != "listener is registered twice" {
		t.Fatalf("conclusion=%q", conclusion)
	}
	if len(findings) != 1 || findings[0].Claim != "reconnect duplicates the listener" || findings[0].Confidence != "high" {
		t.Fatalf("findings=%#v", findings)
	}
	if len(findings[0].Evidence) != 1 || findings[0].Evidence[0].Target != "session.go" {
		t.Fatalf("validated evidence=%#v", findings[0].Evidence)
	}
	if len(blockers) != 1 || blockers[0] != "needs integration test" {
		t.Fatalf("blockers=%#v", blockers)
	}
}

func TestParseChildSemanticResultMalformedEnvelopeUsesVisiblePrefix(t *testing.T) {
	content := "usable conclusion\n<proton-subagent-result>{not-json}</proton-subagent-result>"
	conclusion, findings, blockers := parseChildSemanticResult(content, nil)
	if conclusion != "usable conclusion" {
		t.Fatalf("conclusion=%q", conclusion)
	}
	if len(findings) != 0 || len(blockers) != 0 {
		t.Fatalf("unexpected semantic payload: %#v %#v", findings, blockers)
	}
}

func TestParseChildSemanticResultBoundsClaimsAndBlockers(t *testing.T) {
	claim := strings.Repeat("c", maxFindingClaimBytes+100)
	blocker := strings.Repeat("b", maxStructuredBlockerBytes+100)
	content := childResultOpenTag + `{"conclusion":"done","findings":[{"claim":"` + claim + `"}],"blockers":["` + blocker + `"]}` + childResultCloseTag
	_, findings, blockers := parseChildSemanticResult(content, nil)
	if len(findings) != 1 || len(findings[0].Claim) > maxFindingClaimBytes {
		t.Fatalf("bounded findings=%#v", findings)
	}
	if len(blockers) != 1 || len(blockers[0]) > maxStructuredBlockerBytes {
		t.Fatalf("bounded blockers=%#v", blockers)
	}
}
