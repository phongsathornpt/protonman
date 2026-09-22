package execview

import (
	"strings"
	"testing"
)

func TestPluralCount(t *testing.T) {
	tests := []struct {
		n      int
		sing   string
		plural string
		want   string
	}{
		{0, "failure", "failures", "0 failures"},
		{1, "failure", "failures", "1 failure"},
		{2, "failure", "failures", "2 failures"},
		{1, "error", "errors", "1 error"},
		{2, "error", "errors", "2 errors"},
		{1, "passed", "passed", "1 passed"},
	}
	for _, tt := range tests {
		if got := pluralCount(tt.n, tt.sing, tt.plural); got != tt.want {
			t.Errorf("pluralCount(%d, %q, %q) = %q, want %q", tt.n, tt.sing, tt.plural, got, tt.want)
		}
	}
}

func TestExecSummaryPrimitives(t *testing.T) {
	if got := formatTestCounts(testCounts{Passed: 8, Failed: 1, Skipped: 2}); got != "8 passed · 1 failed · 2 skipped" {
		t.Fatalf("formatTestCounts = %q", got)
	}
	if got := formatDiagnosticCounts(diagnosticCounts{Errors: 2, Warnings: 3}); got != "2 errors · 3 warnings" {
		t.Fatalf("formatDiagnosticCounts = %q", got)
	}
	lines := firstFailureLines("ok\nFAIL parser::nested\nerror[E1]: bad\n", 2)
	if got := strings.Join(lines, "\n"); !strings.Contains(got, "FAIL parser::nested") || !strings.Contains(got, "error[E1]") {
		t.Fatalf("firstFailureLines = %#v", lines)
	}
}

func TestAttachFailureDetails(t *testing.T) {
	p := &Presentation{}
	attachFailureDetails(p, "ok\nFAIL one\nerror two\npanic: three\nfailure four\n")
	want := []string{"FAIL one", "error two", "panic: three"}
	if strings.Join(p.Details, "\n") != strings.Join(want, "\n") {
		t.Fatalf("Details = %#v, want %#v", p.Details, want)
	}

	p = &Presentation{}
	attachFailureDetails(p, "all tests passed\nnothing to report\n")
	if len(p.Details) != 0 {
		t.Fatalf("Details = %#v, want empty", p.Details)
	}
}

func TestAtoiExecReportsParseFailure(t *testing.T) {
	if n, ok := atoiExec("42"); !ok || n != 42 {
		t.Fatalf("atoiExec(\"42\") = %d, %v; want 42, true", n, ok)
	}
	// 25 digits overflows int on every supported platform; strconv must
	// report the failure instead of yielding a fabricated MaxInt count.
	if n, ok := atoiExec("1234567890123456789012345"); ok {
		t.Fatalf("atoiExec(25-digit value) = %d, true; want _, false", n)
	}
}
