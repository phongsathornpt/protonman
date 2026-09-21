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

func TestDiffStatsIgnoresIndentedContextWithSigns(t *testing.T) {
	// Markdown list items or code with -5, --i, ++x in context lines must NOT be counted as edits.
	diff := strings.Join([]string{
		"diff --git a/doc.md b/doc.md",
		"--- a/doc.md",
		"+++ b/doc.md",
		"@@ -1,6 +1,6 @@",
		" - existing markdown bullet 1",
		"   - nested bullet 2",
		"   --i; decrement operator",
		"-old line",
		"+new line",
		"   ++count; increment operator",
		"   +42; unary plus",
	}, "\n")

	adds, dels := DiffStats(diff)
	if adds != 1 {
		t.Fatalf("expected 1 addition, got %d", adds)
	}
	if dels != 1 {
		t.Fatalf("expected 1 deletion, got %d", dels)
	}
}

func TestCleanDiffPathPreservesRealDirectories(t *testing.T) {
	// Real directory paths starting with a/ or b/ must not have their directories stripped.
	diffA := GitDiffFile("a/main.go", "a/main.go", "package a\n", "package a\n// modified\n", 3)
	if !strings.Contains(diffA, "diff --git a/a/main.go b/a/main.go") {
		t.Fatalf("expected path to retain 'a/main.go', got:\n%s", diffA)
	}

	diffNested := GitDiffFile("a/b/c.go", "a/b/c.go", "package c\n", "package c\n// mod\n", 3)
	if !strings.Contains(diffNested, "diff --git a/a/b/c.go b/a/b/c.go") {
		t.Fatalf("expected path to retain 'a/b/c.go', got:\n%s", diffNested)
	}

	// But git diff headers with "a/foo" and "b/foo" should strip the git-diff prefix
	diffGitPrefix := GitDiffFile("a/foo.go", "b/foo.go", "x\n", "y\n", 3)
	if !strings.Contains(diffGitPrefix, "diff --git a/foo.go b/foo.go") {
		t.Fatalf("expected git prefixes to be recognized for same file, got:\n%s", diffGitPrefix)
	}
}

func TestNewFileDiff(t *testing.T) {
	// Empty new file
	emptyDiff := NewFileDiff("empty.txt", "", 3)
	if !strings.Contains(emptyDiff, "diff --git a/empty.txt b/empty.txt") {
		t.Fatalf("expected diff --git header, got:\n%s", emptyDiff)
	}
	if !strings.Contains(emptyDiff, "new file mode 100644") {
		t.Fatalf("expected new file mode header, got:\n%s", emptyDiff)
	}
	if !strings.Contains(emptyDiff, "index 0000000.."+EmptyBlobHash) {
		t.Fatalf("expected index 0000000..%s, got:\n%s", EmptyBlobHash, emptyDiff)
	}
	if !strings.HasSuffix(emptyDiff, "\n") {
		t.Fatalf("expected trailing newline in empty diff, got %q", emptyDiff)
	}

	// Non-empty new file
	contentDiff := NewFileDiff("hello.txt", "hello world\n", 3)
	if !strings.Contains(contentDiff, "new file mode 100644") {
		t.Fatalf("expected new file mode, got:\n%s", contentDiff)
	}
	if !strings.Contains(contentDiff, "+hello world") {
		t.Fatalf("expected addition line, got:\n%s", contentDiff)
	}
	if !strings.HasSuffix(contentDiff, "\n") {
		t.Fatalf("expected trailing newline, got %q", contentDiff)
	}
}

func TestExtractPreviewWithoutHunks(t *testing.T) {
	// When diff has no @@ hunks (e.g. pure rename), ExtractPreview should return lines without breaking
	renameDiff := strings.Join([]string{
		"diff --git a/old.go b/new.go",
		"similarity index 100%",
		"rename from old.go",
		"rename to new.go",
	}, "\n")

	preview, remaining := ExtractPreview(renameDiff, 2)
	if len(preview) != 2 {
		t.Fatalf("expected 2 preview lines, got %d", len(preview))
	}
	if preview[0] != "diff --git a/old.go b/new.go" {
		t.Fatalf("expected first line to be diff header, got %q", preview[0])
	}
	if remaining != 2 {
		t.Fatalf("expected 2 remaining lines, got %d", remaining)
	}
}
