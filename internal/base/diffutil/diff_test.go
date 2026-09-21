package diffutil

import (
	"strings"
	"testing"
)

func TestUnifiedDiff(t *testing.T) {
	orig := "func Hello() string {\n\treturn \"world\"\n}\n"
	mod := "func Hello() string {\n\treturn \"Protonman\"\n}\n"

	diff := UnifiedDiff(orig, mod, "hello.go", 1)
	if !strings.Contains(diff, "diff --git a/hello.go b/hello.go") {
		t.Fatalf("expected diff --git header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "index ") {
		t.Fatalf("expected index line, got:\n%s", diff)
	}
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

func TestUnifiedDiffNewFile(t *testing.T) {
	mod := "package main\n\nfunc main() {}\n"
	diff := UnifiedDiff("", mod, "new.go", 3)
	if !strings.Contains(diff, "diff --git a/new.go b/new.go") {
		t.Fatalf("expected diff --git header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "new file mode 100644") {
		t.Fatalf("expected new file mode header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "index 0000000..") {
		t.Fatalf("expected index 0000000.. header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "--- /dev/null") {
		t.Fatalf("expected --- /dev/null, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+++ b/new.go") {
		t.Fatalf("expected +++ b/new.go, got:\n%s", diff)
	}
	adds, dels := DiffStats(diff)
	if adds != 3 || dels != 0 {
		t.Fatalf("expected +3 -0, got +%d -%d", adds, dels)
	}
}

func TestUnifiedDiffDeletedFile(t *testing.T) {
	orig := "package main\n\nfunc main() {}\n"
	diff := UnifiedDiff(orig, "", "deleted.go", 3)
	if !strings.Contains(diff, "diff --git a/deleted.go b/deleted.go") {
		t.Fatalf("expected diff --git header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "deleted file mode 100644") {
		t.Fatalf("expected deleted file mode header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "..0000000") {
		t.Fatalf("expected ..0000000 in index line, got:\n%s", diff)
	}
	if !strings.Contains(diff, "--- a/deleted.go") {
		t.Fatalf("expected --- a/deleted.go, got:\n%s", diff)
	}
	if !strings.Contains(diff, "+++ /dev/null") {
		t.Fatalf("expected +++ /dev/null, got:\n%s", diff)
	}
	adds, dels := DiffStats(diff)
	if adds != 0 || dels != 3 {
		t.Fatalf("expected +0 -3, got +%d -%d", adds, dels)
	}
}

func TestGitDiffRename(t *testing.T) {
	content := "package main\n"
	diff := GitDiffFile("old.go", "new.go", content, content, 3)
	if !strings.Contains(diff, "diff --git a/old.go b/new.go") {
		t.Fatalf("expected diff --git header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "similarity index 100%") {
		t.Fatalf("expected similarity index header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "rename from old.go") {
		t.Fatalf("expected rename from header, got:\n%s", diff)
	}
	if !strings.Contains(diff, "rename to new.go") {
		t.Fatalf("expected rename to header, got:\n%s", diff)
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
