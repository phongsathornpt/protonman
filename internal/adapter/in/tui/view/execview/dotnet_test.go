package execview

import "testing"

func TestDotnetTestCountOverflowKeepsRawOutput(t *testing.T) {
	p := Present("dotnet test", "Failed: 0, Passed: 1234567890123456789012345, Skipped: 0", "")
	if p.SuppressRaw {
		t.Error("SuppressRaw = true, want false so raw output stays visible")
	}
	if p.Summary != "" {
		t.Errorf("Summary = %q, want empty (no fabricated count)", p.Summary)
	}
}

func TestDotnetTestSummaryStillParsesValidCounts(t *testing.T) {
	p := Present("dotnet test", "Failed: 0, Passed: 3, Skipped: 1", "")
	if p.Summary != "3 passed · 1 skipped" {
		t.Errorf("Summary = %q, want %q", p.Summary, "3 passed · 1 skipped")
	}
	if !p.SuppressRaw {
		t.Error("SuppressRaw = false, want true")
	}
}
