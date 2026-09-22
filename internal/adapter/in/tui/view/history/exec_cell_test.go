package history

import (
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestIsDiffCommand(t *testing.T) {
	// Commands that MUST be recognized as diff commands
	diffCmds := []string{
		"diff -u a.txt b.txt",
		"/usr/bin/diff file1 file2",
		"colordiff old.go new.go",
		"patch -p1 < fix.patch",
		"git diff",
		"git diff HEAD~1",
		"git --no-pager diff main",
		"git -C /workspace diff",
		"git show abc1234",
		"git log -p -2",
		"git log --patch",
		"svn diff",
		"git diff | head -n 20",
	}
	for _, cmd := range diffCmds {
		if !isDiffCommand(cmd) {
			t.Errorf("expected isDiffCommand(%q) = true, got false", cmd)
		}
	}

	// Commands that must NOT be false-positived merely by containing "diff" or "patch" as substrings
	nonDiffCmds := []string{
		"go test ./internal/base/diffutil/...",
		"python run_dispatcher.py",
		"pytest tests/test_diff_engine.py",
		"npm run build:different",
		"cat patchwork.txt",
		"echo 'difficult situation'",
		"ls -la /tmp/patches",
		"git status",
		"git commit -m 'fix diff bug'",
		"git checkout -b feature/diff-tool",
	}
	for _, cmd := range nonDiffCmds {
		if isDiffCommand(cmd) {
			t.Errorf("expected isDiffCommand(%q) = false, got true", cmd)
		}
	}
}

func TestIsDiffOutput(t *testing.T) {
	// Output with diff headers must be detected even if command is generic
	linesWithHeaders := []string{
		"diff --git a/foo.go b/foo.go",
		"--- a/foo.go",
		"+++ b/foo.go",
		"@@ -1,3 +1,4 @@",
		"+new line",
	}
	if !isDiffOutput("cat changes.txt", linesWithHeaders) {
		t.Error("expected isDiffOutput = true for output containing diff --git headers")
	}

	// Output without diff headers and non-diff command must be false
	plainLines := []string{
		"PASS",
		"ok  	github.com/phongsathornpt/protonman/internal/base/diffutil	0.123s",
		"- item 1",
		"+ item 2",
	}
	if isDiffOutput("go test ./internal/base/diffutil/...", plainLines) {
		t.Error("expected isDiffOutput = false for go test output")
	}
}

func TestExecCellRenderedFailureUsesHumanLabel(t *testing.T) {
	cell := ExecCell{
		Name:        "bash",
		Command:     "go test ./...",
		Stdout:      "some output",
		FailureCode: tool.ErrorCodeDeadlineExceeded,
	}
	rendered := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(rendered, "timed out") {
		t.Fatalf("rendered failure label missing:\n%s", rendered)
	}
	if strings.Contains(rendered, "deadline_exceeded") {
		t.Fatalf("rendered failure leaked raw code:\n%s", rendered)
	}
}

func TestExecCellRawLinesKeepStructuredCode(t *testing.T) {
	cell := ExecCell{
		Name:        "bash",
		Command:     "go test ./...",
		Stdout:      "some output",
		FailureCode: tool.ErrorCodeDeadlineExceeded,
	}
	raw := strings.Join(cell.RawLines(), "\n")
	if !strings.Contains(raw, "deadline_exceeded") {
		t.Fatalf("raw lines must keep the structured code for diagnostics:\n%s", raw)
	}
}
