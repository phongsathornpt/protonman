package history

import (
	"strings"
	"testing"

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
}
