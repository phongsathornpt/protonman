package execview

import "testing"

func TestSummarizeNodeTestSuppressRaw(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantSuppress bool
		wantSummary  string
	}{
		{
			// No "# fail N" line means the failure count is the -1 sentinel:
			// the parse did not establish a clean result, so raw output stays.
			name:         "unknown failure count keeps raw output",
			output:       "# tests 3\n# pass 3\n",
			wantSuppress: false,
			wantSummary:  "3 passed",
		},
		{
			name:         "zero failures suppress raw output",
			output:       "# tests 3\n# pass 3\n# fail 0\n",
			wantSuppress: true,
			wantSummary:  "3 passed",
		},
		{
			name:         "failures keep raw output",
			output:       "# tests 3\n# pass 2\n# fail 1\n",
			wantSuppress: false,
			wantSummary:  "2 passed · 1 failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Presentation{Action: "test"}
			summarizeNodeExec(&p, tt.output)
			if p.SuppressRaw != tt.wantSuppress {
				t.Errorf("SuppressRaw = %v, want %v (summary %q)", p.SuppressRaw, tt.wantSuppress, p.Summary)
			}
			if p.Summary != tt.wantSummary {
				t.Errorf("Summary = %q, want %q", p.Summary, tt.wantSummary)
			}
		})
	}
}

func TestSummarizeBunTestSuppressRaw(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantSuppress bool
		wantSummary  string
	}{
		{
			// No "N fail" line means the failure count is the -1 sentinel.
			name:         "unknown failure count keeps raw output",
			output:       "3 pass\n",
			wantSuppress: false,
			wantSummary:  "3 passed",
		},
		{
			name:         "zero failures suppress raw output",
			output:       "3 pass\n0 fail\n",
			wantSuppress: true,
			wantSummary:  "3 passed",
		},
		{
			name:         "failures keep raw output",
			output:       "2 pass\n1 fail\n",
			wantSuppress: false,
			wantSummary:  "2 passed · 1 failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Presentation{Action: "test"}
			summarizeBunExec(&p, tt.output)
			if p.SuppressRaw != tt.wantSuppress {
				t.Errorf("SuppressRaw = %v, want %v (summary %q)", p.SuppressRaw, tt.wantSuppress, p.Summary)
			}
			if p.Summary != tt.wantSummary {
				t.Errorf("Summary = %q, want %q", p.Summary, tt.wantSummary)
			}
		})
	}
}
