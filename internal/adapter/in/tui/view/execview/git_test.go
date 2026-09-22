package execview

import (
	"strings"
	"testing"
)

func TestSummarizeGitDiffStat(t *testing.T) {
	p := Presentation{Action: "diff"}
	summarizeGitExec(&p, "3 files changed, 10 insertions(+), 2 deletions(-)")
	if p.Summary != "3 files · +10 -2" {
		t.Fatalf("summary = %q, want %q", p.Summary, "3 files · +10 -2")
	}
	if !p.SuppressRaw {
		t.Fatal("expected raw output suppressed on a valid stat")
	}
}

func TestSummarizeGitDiffStatWithoutInsertionGroups(t *testing.T) {
	p := Presentation{Action: "diff"}
	summarizeGitExec(&p, "2 files changed")
	if p.Summary != "2 files" {
		t.Fatalf("summary = %q, want %q", p.Summary, "2 files")
	}
}

func TestSummarizeGitDiffStatOverflowKeepsRawOutput(t *testing.T) {
	p := Presentation{Action: "diff"}
	summarizeGitExec(&p, "99999999999999999999 files changed, 1 insertion(+), 2 deletions(-)")
	if p.SuppressRaw {
		t.Fatal("overflowing stat must not suppress raw output")
	}
	if strings.Contains(p.Summary, "99999999999999999999") || p.Summary != "" {
		t.Fatalf("overflowing stat must not fabricate a summary, got %q", p.Summary)
	}
}
