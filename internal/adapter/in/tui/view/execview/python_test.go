package execview

import "testing"

func TestSummarizePytestLabelsAndSuppression(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantSuppress bool
		wantSummary  string
	}{
		{
			name:         "zero failures suppress raw output",
			output:       "=== 3 passed in 0.11s ===",
			wantSuppress: true,
			wantSummary:  "3 passed",
		},
		{
			name:         "singular error label",
			output:       "=== 1 passed, 1 error in 0.12s ===",
			wantSuppress: false,
			wantSummary:  "1 passed · 1 error",
		},
		{
			name:         "plural errors label",
			output:       "=== 2 passed, 2 errors in 0.13s ===",
			wantSuppress: false,
			wantSummary:  "2 passed · 2 errors",
		},
		{
			name:         "failures keep raw output",
			output:       "=== 2 passed, 1 failed in 0.14s ===",
			wantSuppress: false,
			wantSummary:  "2 passed · 1 failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Presentation{Action: "pytest"}
			summarizePytestExec(&p, tt.output)
			if p.SuppressRaw != tt.wantSuppress {
				t.Errorf("SuppressRaw = %v, want %v (summary %q)", p.SuppressRaw, tt.wantSuppress, p.Summary)
			}
			if p.Summary != tt.wantSummary {
				t.Errorf("Summary = %q, want %q", p.Summary, tt.wantSummary)
			}
		})
	}
}

func TestSummarizeUnittestLabelsAndSuppression(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantSuppress bool
		wantSummary  string
	}{
		{
			name:         "zero failures suppress raw output",
			output:       "Ran 2 tests in 0.001s\n\nOK\n",
			wantSuppress: true,
			wantSummary:  "2 passed",
		},
		{
			name:         "singular error label",
			output:       "Ran 2 tests in 0.001s\n\nFAILED (errors=1)\n",
			wantSuppress: false,
			wantSummary:  "1 passed · 1 error",
		},
		{
			name:         "plural errors label",
			output:       "Ran 3 tests in 0.001s\n\nFAILED (errors=2)\n",
			wantSuppress: false,
			wantSummary:  "1 passed · 2 errors",
		},
		{
			name:         "failures keep raw output",
			output:       "Ran 2 tests in 0.001s\n\nFAILED (failures=1)\n",
			wantSuppress: false,
			wantSummary:  "1 passed · 1 failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Presentation{Action: "unittest"}
			summarizeUnittestExec(&p, tt.output)
			if p.SuppressRaw != tt.wantSuppress {
				t.Errorf("SuppressRaw = %v, want %v (summary %q)", p.SuppressRaw, tt.wantSuppress, p.Summary)
			}
			if p.Summary != tt.wantSummary {
				t.Errorf("Summary = %q, want %q", p.Summary, tt.wantSummary)
			}
		})
	}
}
