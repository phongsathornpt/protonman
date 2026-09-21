package diffutil

import (
	"strings"
	"testing"
)

func TestUnifiedDiff(t *testing.T) {
	orig := "func Hello() string {\n\treturn \"world\"\n}\n"
	mod := "func Hello() string {\n\treturn \"Protonman\"\n}\n"

	diff := UnifiedDiff(orig, mod, "hello.go", 1)
	if !strings.Contains(diff, "--- a/hello.go") {
		t.Fatalf("expected --- header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+++ b/hello.go") {
		t.Fatalf("expected +++ header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "-	return \"world\"") {
		t.Fatalf("expected deletion line, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+	return \"Protonman\"") {
		t.Fatalf("expected addition line, got:\n%s", diff)
	}

	adds, dels := DiffStats(diff)
	if adds != 1 || dels != 1 {
		t.Fatalf("expected 1 addition and 1 deletion, got +%d -%d", adds, dels)
	}
	badge := StatBadge(adds, dels)
	if badge != "+1 -1" {
		t.Fatalf("expected badge '+1 -1', got %q", badge)
	}
}

func TestUnifiedDiffIdentical(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	diff := UnifiedDiff(content, content, "main.go", 3)
	if diff != "" {
		t.Fatalf("expected empty diff for identical inputs, got:\n%s", diff)
	}
	adds, dels := DiffStats(diff)
	if adds != 0 || dels != 0 {
		t.Fatalf("expected 0 stats, got +%d -%d", adds, dels)
	}
	if StatBadge(adds, dels) != "" {
		t.Fatalf("expected empty badge for 0 stats, got %q", StatBadge(adds, dels))
	}
}

func TestExtractPreview(t *testing.T) {
	diff := strings.Join([]string{
		"--- a/foo.go",
		"+++ b/foo.go",
		"@@ -1,5 +1,6 @@",
		" line 1",
		"-line 2",
		"+line 2 modified",
		"+line 2.5 added",
		" line 3",
		" line 4",
	}, "\n")

	preview, remaining := ExtractPreview(diff, 4)
	if len(preview) != 4 {
		t.Fatalf("expected 4 preview lines, got %d", len(preview))
	}
	if preview[0] != "@@ -1,5 +1,6 @@" {
		t.Fatalf("expected hunk header as first preview line, got %q", preview[0])
	}
	if remaining != 3 {
		t.Fatalf("expected 3 remaining lines, got %d", remaining)
	}
}

func TestDiffStatsExcludesHeaders(t *testing.T) {
	diff := strings.Join([]string{
		"--- a/foo.go",
		"+++ b/foo.go",
		"@@ -1,2 +1,3 @@",
		"-old",
		"+new",
		"+extra",
	}, "\n")

	adds, dels := DiffStats(diff)
	if adds != 2 {
		t.Fatalf("expected 2 additions, got %d", adds)
	}
	if dels != 1 {
		t.Fatalf("expected 1 deletion, got %d", dels)
	}
}
