package execview

import (
	"strings"
	"testing"
)

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
