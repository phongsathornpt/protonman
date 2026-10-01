//go:build desktop || desktop_gio

package shell

import (
	"testing"
)

func TestHighlightCodeSpans(t *testing.T) {
	th := NewTheme("dark")
	spans := highlightCodeSpans(th, "go", "package main\n\nfunc main() {}\n")
	if len(spans) == 0 {
		t.Fatal("highlightCodeSpans returned empty spans")
	}
	foundPkg := false
	for _, s := range spans {
		if s.Content == "package" {
			foundPkg = true
			if s.Color != th.Colors.Primary {
				t.Errorf("package keyword color = %v, want %v", s.Color, th.Colors.Primary)
			}
		}
	}
	if !foundPkg {
		t.Errorf("highlightCodeSpans did not find 'package' span: %#v", spans)
	}
}
