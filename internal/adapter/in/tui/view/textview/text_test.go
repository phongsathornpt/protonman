package textview

import (
	"strings"
	"testing"
)

func TestWrapLinesBreaksAtSlashForLongPaths(t *testing.T) {
	// Path with length 47, width 45.
	// Without slash breaking, "update_runtime" would split as "update_runtim" and "e.go".
	path := "internal/adapter/in/tui/runtime/update_runtime.go"
	lines := WrapLines(path, 45)

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %#v", len(lines), lines)
	}
	if lines[0] != "internal/adapter/in/tui/runtime/" {
		t.Errorf("line 0 = %q, want 'internal/adapter/in/tui/runtime/'", lines[0])
	}
	if lines[1] != "update_runtime.go" {
		t.Errorf("line 1 = %q, want 'update_runtime.go'", lines[1])
	}
}

func TestWrapLinesSentenceWithLongFilePath(t *testing.T) {
	sentence := "The file internal/adapter/in/tui/runtime/update_runtime.go has been updated."
	lines := WrapLines(sentence, 45)

	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "update_runtim\n") || strings.Contains(joined, "\ne.go") || strings.Contains(joined, "\n.go") {
		t.Fatalf("file path extension broken across lines:\n%s", joined)
	}

	// Verify that the filename update_runtime.go is kept contiguous on one line.
	foundContiguous := false
	for _, line := range lines {
		if strings.Contains(line, "update_runtime.go") {
			foundContiguous = true
			break
		}
	}
	if !foundContiguous {
		t.Fatalf("update_runtime.go was not kept contiguous in lines: %#v", lines)
	}
}
