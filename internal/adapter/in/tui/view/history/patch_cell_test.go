package history

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestPatchCellRetryPresentation(t *testing.T) {
	// 1. Initial running state: plan style, no retry indicator
	initial := &PatchCell{
		CallID:   "call-1",
		Name:     "edit",
		Paths:    []string{"update_runtime.go"},
		Attempts: 1,
		Running:  true,
		Icons:    tuistyle.ASCIIIcons,
	}
	initialLines := initial.RenderWidth(80)
	if len(initialLines) == 0 {
		t.Fatal("expected rendered lines for initial cell")
	}
	if strings.Contains(initialLines[0], "retrying") {
		t.Fatalf("initial attempt should not say retrying: %q", initialLines[0])
	}

	// 2. Failed attempt waiting for retry: warning style, attempt count, excerpt
	retrying := &PatchCell{
		CallID:      "call-1",
		Name:        "edit",
		Paths:       []string{"update_runtime.go"},
		Attempts:    1,
		Retrying:    true,
		FailureCode: tool.ErrorCodeExecution,
		LastError:   "execute edit: oldString was not found in \"update_runtime.go\"",
		Icons:       tuistyle.ASCIIIcons,
	}
	retryingLines := retrying.RenderWidth(80)
	if len(retryingLines) < 2 {
		t.Fatalf("expected header and excerpt, got %d lines: %#v", len(retryingLines), retryingLines)
	}
	if !strings.Contains(retryingLines[0], "retrying") {
		t.Fatalf("expected retrying indicator in header: %q", retryingLines[0])
	}
	if !strings.Contains(retryingLines[1], "oldString was not found") {
		t.Fatalf("expected failure excerpt: %q", retryingLines[1])
	}

	// 3. Running second attempt: shows attempt 2
	attempt2 := &PatchCell{
		CallID:   "call-2",
		Name:     "edit",
		Paths:    []string{"update_runtime.go"},
		Attempts: 2,
		Running:  true,
		Retrying: true,
		Icons:    tuistyle.ASCIIIcons,
	}
	attempt2Lines := attempt2.RenderWidth(80)
	if !strings.Contains(attempt2Lines[0], "attempt 2") {
		t.Fatalf("expected 'attempt 2' in header: %q", attempt2Lines[0])
	}

	// 4. Successful retry: shows (retried 1x)
	success := &PatchCell{
		CallID:   "call-2",
		Name:     "edit",
		Paths:    []string{"update_runtime.go"},
		Attempts: 2,
		Running:  false,
		Retrying: false,
		Icons:    tuistyle.ASCIIIcons,
	}
	successLines := success.RenderWidth(80)
	if !strings.Contains(successLines[0], "retried 1x") {
		t.Fatalf("expected (retried 1x) in header: %q", successLines[0])
	}

	// 5. Final failure after multiple attempts: shows failure count and error excerpt
	finalFailure := &PatchCell{
		CallID:      "call-2",
		Name:        "edit",
		Paths:       []string{"update_runtime.go"},
		Attempts:    2,
		Running:     false,
		Retrying:    false,
		FailureCode: tool.ErrorCodeExecution,
		LastError:   "execute edit: oldString was not found in \"update_runtime.go\"",
		Icons:       tuistyle.ASCIIIcons,
	}
	finalFailureLines := finalFailure.RenderWidth(80)
	if len(finalFailureLines) < 2 {
		t.Fatalf("expected header and error excerpt on terminal failure, got %d lines: %#v", len(finalFailureLines), finalFailureLines)
	}
	if !strings.Contains(finalFailureLines[0], "failed after 2 attempts") {
		t.Fatalf("expected 'failed after 2 attempts' in header: %q", finalFailureLines[0])
	}
	if !strings.Contains(finalFailureLines[0], "execution failed") {
		t.Fatalf("expected human-facing 'execution failed' in header: %q", finalFailureLines[0])
	}
	if !strings.Contains(finalFailureLines[1], "oldString was not found") {
		t.Fatalf("expected error excerpt in line 1 on terminal failure, got: %q", finalFailureLines[1])
	}

	// 6. Successful edit with CheckpointID
	cellSuccessCP := &PatchCell{
		Name:         "edit",
		Paths:        []string{"main.go"},
		CheckpointID: "checkpoint-1789386423434583000-67a4fe9f13ee78a",
		Icons:        tuistyle.UnicodeIcons,
	}
	cpLines := cellSuccessCP.RenderWidth(80)
	if len(cpLines) < 2 {
		t.Fatalf("expected header and checkpoint line, got %d lines: %#v", len(cpLines), cpLines)
	}
	if !strings.Contains(cpLines[1], "checkpoint chk-67a4fe9f") {
		t.Fatalf("expected compact checkpoint line, got: %q", cpLines[1])
	}

	// 7. Narrow width path formatting preserves single line without awkward multi-line break
	narrowCell := &PatchCell{
		Name:    "edit",
		Paths:   []string{"internal/adapter/in/tui/runtime/keyboard_runtime.go"},
		Running: true,
		Icons:   tuistyle.UnicodeIcons,
	}
	narrowLines := narrowCell.RenderWidth(40)
	if len(narrowLines) == 0 {
		t.Fatal("expected narrow lines")
	}
	if !strings.Contains(narrowLines[0], "keyboard_runtime.go") {
		t.Fatalf("expected target filename preserved in narrow header: %q", narrowLines[0])
	}

	// 8. Multi-line error excerpt uses 4-space aligned hanging indent
	multiLineErrCell := &PatchCell{
		Name:        "edit",
		Paths:       []string{"main.go"},
		FailureCode: tool.ErrorCodeExecution,
		LastError:   "first line of error message that is long enough to wrap onto a second line for testing",
		Icons:       tuistyle.UnicodeIcons,
	}
	// Wrap at 40 columns to force error to wrap
	wrappedErrLines := multiLineErrCell.RenderWidth(40)
	if len(wrappedErrLines) < 3 {
		t.Fatalf("expected header and at least 2 wrapped error lines, got %d: %#v", len(wrappedErrLines), wrappedErrLines)
	}
	if !strings.HasPrefix(ansi.Strip(wrappedErrLines[1]), "  ↳ ") {
		t.Errorf("line 1 want prefix '  ↳ ', got %q", ansi.Strip(wrappedErrLines[1]))
	}
	if !strings.HasPrefix(ansi.Strip(wrappedErrLines[2]), "    ") {
		t.Errorf("continuation line 2 want prefix '    ' (4 spaces hanging indent), got %q", ansi.Strip(wrappedErrLines[2]))
	}

	// 9. RawLines conveys status
	runningRaw := (&PatchCell{Name: "edit", Running: true}).RawLines()
	if len(runningRaw) == 0 || !strings.Contains(runningRaw[0], "[running]") {
		t.Errorf("running RawLines expected '[running]', got: %#v", runningRaw)
	}
	failedRaw := (&PatchCell{Name: "edit", FailureCode: tool.ErrorCodeExecution}).RawLines()
	if len(failedRaw) == 0 || !strings.Contains(failedRaw[0], "[execution_error]") {
		t.Errorf("failed RawLines expected '[execution_error]', got: %#v", failedRaw)
	}
	deniedRaw := (&PatchCell{Name: "edit", Denied: true}).RawLines()
	if len(deniedRaw) == 0 || !strings.Contains(deniedRaw[0], "[denied]") {
		t.Errorf("denied RawLines expected '[denied]', got: %#v", deniedRaw)
	}
}

func TestPatchCellVisualDiffPresentation(t *testing.T) {
	diffContent := strings.Join([]string{
		"diff --git a/service.go b/service.go",
		"index 1234567..89abcdef 100644",
		"--- a/service.go",
		"+++ b/service.go",
		"@@ -10,3 +10,5 @@ func Init() error {",
		"   config := loadConfig()",
		"-  return start(config)",
		"+  if err := validate(config); err != nil {",
		"+    return err",
		"+  }",
		"+  return start(config)",
		" }",
		" // more lines",
		" // extra lines to test folding",
	}, "\n")

	cell := &PatchCell{
		CallID:    "call-diff",
		Name:      "edit",
		Paths:     []string{"service.go"},
		Additions: 4,
		Deletions: 1,
		Diff:      diffContent,
		Icons:     tuistyle.UnicodeIcons,
	}

	rendered := cell.RenderWidth(80)
	joined := ansi.Strip(strings.Join(rendered, "\n"))

	// 1. Header has stat badge +4 -1
	if !strings.Contains(joined, "+4 -1") {
		t.Fatalf("expected '+4 -1' badge in header, got:\n%s", joined)
	}

	// 2. Diff hunk header and added/deleted lines are rendered
	if !strings.Contains(joined, "@@ -10,3 +10,5 @@") {
		t.Fatalf("expected hunk header in render, got:\n%s", joined)
	}
	if !strings.Contains(joined, "-  return start(config)") {
		t.Fatalf("expected deletion line in render, got:\n%s", joined)
	}
	if !strings.Contains(joined, "+  if err := validate(config)") {
		t.Fatalf("expected addition line in render, got:\n%s", joined)
	}

	// 3. Fold indicator appears when diff exceeds preview limit (5 lines)
	if !strings.Contains(joined, "more lines · ctrl+t for full diff") {
		t.Fatalf("expected fold hint in render, got:\n%s", joined)
	}

	// 4. RawLines contains full diff including git headers
	rawLines := cell.RawLines()
	rawJoined := strings.Join(rawLines, "\n")
	if !strings.Contains(rawJoined, "diff --git a/service.go b/service.go") {
		t.Fatalf("expected git diff header in raw lines, got:\n%s", rawJoined)
	}
	if !strings.Contains(rawJoined, "+  return start(config)") || !strings.Contains(rawJoined, "// extra lines to test folding") {
		t.Fatalf("expected raw lines to include full diff, got:\n%s", rawJoined)
	}
}
