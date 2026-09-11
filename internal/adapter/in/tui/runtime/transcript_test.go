package runtime

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/transcriptutil"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
	"math"
	"strings"
	"testing"
)

func TestHistoryStateStreamsAssistantIntoActiveCell(t *testing.T) {
	state := NewHistoryState(100)
	state.Append(&UserCell{Text: "hello"})
	state.AppendAssistantDelta("hel")
	state.AppendAssistantDelta("lo")
	active, ok := state.Active().(*AssistantCell)
	if !ok {
		t.Fatalf("active cell = %T, want *AssistantCell", state.Active())
	}
	if active.Text != "hello" {
		t.Fatalf("active text = %q, want hello", active.Text)
	}
	if got := state.Raw(); got != "hello\nhello" {
		t.Fatalf("raw transcript = %q", got)
	}
	state.CommitActive()
	if state.Active() != nil {
		t.Fatalf("active cell = %T after commit, want nil", state.Active())
	}
	if got := len(state.Cells()); got != 2 {
		t.Fatalf("cell count = %d, want 2", got)
	}
}

func TestHistoryStateToolRunningToCompleted(t *testing.T) {
	state := NewHistoryState(100)
	state.StartTool("bash")
	active, ok := state.Active().(*ToolCell)
	if !ok || !active.Running {
		t.Fatalf("active tool = %#v, want running ToolCell", state.Active())
	}
	exitCode := 0
	state.CompleteTool(ToolCell{Name: "bash", Body: "ok", ExitCode: &exitCode})
	if state.Active() != nil {
		t.Fatal("completed tool should be committed")
	}
	cells := state.Cells()
	if len(cells) != 1 {
		t.Fatalf("cell count = %d, want 1", len(cells))
	}
	toolCell, ok := cells[0].(*ToolCell)
	if !ok {
		t.Fatalf("cell = %T, want *ToolCell", cells[0])
	}
	if toolCell.Running {
		t.Fatal("completed tool is still marked running")
	}
	if got := state.Raw(); !strings.Contains(got, "Run\nok\nexit 0") {
		t.Fatalf("raw transcript missing tool result: %q", got)
	}
}

func TestHistoryStateCompletesPreviouslyCommittedParallelTool(t *testing.T) {
	state := NewHistoryState(100)
	state.StartTool("read")
	state.StartTool("grep")
	state.CompleteTool(ToolCell{Name: "read", Body: "contents"})
	cells := state.Cells()
	if len(cells) != 2 {
		t.Fatalf("cell count = %d, want 2", len(cells))
	}
	first, ok := cells[0].(*ToolCell)
	if !ok || first.Running || first.Body != "contents" {
		t.Fatalf("first tool = %#v, want completed read", cells[0])
	}
	second, ok := cells[1].(*ToolCell)
	if !ok || !second.Running || second.Name != "grep" {
		t.Fatalf("second tool = %#v, want active grep", cells[1])
	}
}

func TestHistoryStateUsesCallIDForSameNameParallelTools(t *testing.T) {
	state := NewHistoryState(100)
	state.StartToolCall("read-1", "read")
	state.StartToolCall("read-2", "read")
	state.CompleteTool(ToolCell{CallID: "read-1", Name: "read", Body: "first"})
	cells := state.Cells()
	if len(cells) != 2 {
		t.Fatalf("cell count = %d, want 2", len(cells))
	}
	first, ok := cells[0].(*ToolCell)
	if !ok || first.CallID != "read-1" || first.Running || first.Body != "first" {
		t.Fatalf("first tool = %#v, want completed read-1", cells[0])
	}
	second, ok := cells[1].(*ToolCell)
	if !ok || second.CallID != "read-2" || !second.Running {
		t.Fatalf("second tool = %#v, want active read-2", cells[1])
	}
	state.CompleteTool(ToolCell{CallID: "read-2", Name: "read", Body: "second"})
	cells = state.Cells()
	second = cells[1].(*ToolCell)
	if second.Running || second.Body != "second" {
		t.Fatalf("second tool = %#v, want completed read-2", second)
	}
}

func TestHistoryStateBoundsCommittedScrollback(t *testing.T) {
	state := NewHistoryState(3)
	state.Append(&SystemCell{Text: "one"})
	state.Append(&SystemCell{Text: "two"})
	state.Append(&SystemCell{Text: "three"})
	state.Append(&SystemCell{Text: "four"})
	if got := state.Raw(); got != "two\nthree\nfour" {
		t.Fatalf("trimmed transcript = %q", got)
	}
}

func TestHistoryCellKindEnum(t *testing.T) {
	tests := []struct {
		kind HistoryCellKind
		want string
	}{{HistoryCellUnknown, "unknown"}, {HistoryCellUser, "user"}, {HistoryCellAssistant, "assistant"}, {HistoryCellTool, "tool"}, {HistoryCellSystem, "system"}, {HistoryCellError, "error"}, {HistoryCellKind(99), "unknown"}}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("HistoryCellKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestHistoryStateStartsAssistantOnlyOnFirstDelta(t *testing.T) {
	state := NewHistoryState(100)
	if active := state.Active(); active != nil {
		t.Fatalf("active cell before model output = %T, want nil", active)
	}
	state.AppendAssistantDelta("Hello world")
	assistant, ok := state.Active().(*AssistantCell)
	if !ok {
		t.Fatalf("active cell = %T, want *AssistantCell", state.Active())
	}
	if assistant.Text != "Hello world" {
		t.Fatalf("assistant text = %q, want Hello world", assistant.Text)
	}
}

func TestActivateSkillToolCellCompactRendering(t *testing.T) {
	xmlBody := `<skill_content name="golang-performance">
**Persona:** You are a Go performance engineer.
# Go Performance Optimization
1. Profile before optimizing...
</skill_content>`
	cell := ToolCell{Name: "skill", Body: xmlBody}
	raw := cell.RawLines()
	for _, line := range raw {
		if strings.Contains(line, "Profile before optimizing") {
			t.Fatalf("RawLines should not contain raw instruction markdown, got: %v", raw)
		}
	}
	rendered := cell.RenderWidth(80)
	joined := testPlain(strings.Join(rendered, "\n"))
	if !strings.Contains(joined, `Activated skill "golang-performance"`) {
		t.Fatalf("expected compact activation badge in render, got: %s", joined)
	}
	if strings.Contains(joined, "Profile before optimizing") {
		t.Fatalf("rendered output contains full skill instructions: %s", joined)
	}
}

func TestLoadInitialMessagesCompactsSkillDetail(t *testing.T) {
	bm := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "")
	bm.loadInitialMessages([]model.Message{{Role: model.RoleTool, ToolName: "skill", Content: `<skill_content name="golang-code-style">\n# Full instructions...\n</skill_content>`}, {Role: model.RoleUser, Content: "Activated skill pdf-tool [user]:\n# PDF Guide\nLong content here..."}})
	rendered := testPlain(strings.Join(bm.historyState.RenderLines(), "\n"))
	if strings.Contains(rendered, "Full instructions") {
		t.Fatalf("history rendered full skill instructions from tool message: %s", rendered)
	}
	if strings.Contains(rendered, "Long content here") {
		t.Fatalf("history rendered full skill instructions from user message: %s", rendered)
	}
	if !strings.Contains(rendered, `Activated skill "golang-code-style"`) {
		t.Fatalf("history missing compact badge: %s", rendered)
	}
}

func TestToolCellRefinedRenderingWebFetch(t *testing.T) {
	htmlPayload := `<!DOCTYPE html>
<html lang="th">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover" />
    <title>รวม AI ราคาถูกใน API เดียว | protonmanAI</title>
  </head>
  <body><h1>Protonman</h1><p>Many lines of HTML...</p></body>
</html>`
	state := NewHistoryState(100)
	runningCell := &ToolCell{CallID: "call-web-1", Name: "web", Target: "https://protonman.dev", ToolKind: tool.KindWeb, Running: true}
	state.StartToolCell(runningCell)
	rendered := state.RenderLines()
	joinedRunning := strings.Join(rendered, "\n")
	if !strings.Contains(joinedRunning, "↗") || !strings.Contains(joinedRunning, "Web") || !strings.Contains(joinedRunning, "https://protonman.dev") {
		t.Fatalf("expected running cell to show category icon and target, got: %s", joinedRunning)
	}
	completedCell := &ToolCell{CallID: "call-web-1", Name: "web", Target: "https://protonman.dev", ToolKind: tool.KindWeb, Body: htmlPayload, Summary: summarizeToolOutput("web", tool.KindWeb, "https://protonman.dev", htmlPayload, nil, false)}
	state.CompleteToolCall("call-web-1", "web", completedCell)
	rendered = state.RenderLines()
	joinedCompleted := strings.Join(rendered, "\n")
	if !strings.Contains(joinedCompleted, "✓") || !strings.Contains(joinedCompleted, "https://protonman.dev") {
		t.Fatalf("expected completed summary header, got: %s", joinedCompleted)
	}
	if !strings.Contains(joinedCompleted, "รวม AI ราคาถูกใน API เดียว | protonmanAI") {
		t.Fatalf("expected page title in summary, got: %s", joinedCompleted)
	}
	if strings.Contains(joinedCompleted, "<!DOCTYPE html>") || strings.Contains(joinedCompleted, "<body>") {
		t.Fatalf("raw HTML was not suppressed from rendered viewport lines: %s", joinedCompleted)
	}
	raw := state.Raw()
	if !strings.Contains(raw, "<!DOCTYPE html>") || !strings.Contains(raw, "Protonman") {
		t.Fatalf("raw transcript failed to preserve complete HTML payload: %s", raw)
	}
}

func TestToolCellRefinedRenderingReadFile(t *testing.T) {
	fileContent := strings.Repeat("fmt.Println(\"code\")\n", 50)
	cell := &ToolCell{Name: "read", Target: "internal/tui/theme.go", ToolKind: tool.KindRead, Body: fileContent, Summary: summarizeToolOutput("read", tool.KindRead, "internal/tui/theme.go", fileContent, nil, false)}
	rendered := testPlain(strings.Join(cell.RenderWidth(80), "\n"))
	if !strings.Contains(rendered, "50 lines") || !strings.Contains(rendered, "internal/tui/theme.go") {
		t.Fatalf("expected summary with line count and target, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Read") {
		t.Fatalf("expected action verb 'Read' in header, got: %s", rendered)
	}
	if strings.Contains(rendered, "read") {
		t.Fatalf("raw 'read' should be replaced by SSOT DisplayName, got: %s", rendered)
	}
	if strings.Contains(rendered, "fmt.Println") {
		t.Fatalf("raw file contents should be suppressed from viewport, got: %s", rendered)
	}
	raw := strings.Join(cell.RawLines(), "\n")
	if !strings.Contains(raw, "fmt.Println") {
		t.Fatalf("raw transcript missing file content: %s", raw)
	}
	if !strings.HasPrefix(raw, "Read") {
		t.Fatalf("expected RawLines header to start with 'Read', got: %s", raw)
	}
}

func TestExecCellFolding(t *testing.T) {
	exit0 := 0
	longOutput := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\nline 8\n"
	cell := &ExecCell{Command: "npm test", Body: longOutput, ExitCode: &exit0}
	rendered := testPlain(strings.Join(cell.RenderWidth(80), "\n"))
	if !strings.Contains(rendered, "Npm test") || strings.Contains(rendered, "exit 0") {
		t.Fatalf("expected semantic command title without redundant exit 0, got: %s", rendered)
	}
	if !strings.Contains(rendered, "more") || !strings.Contains(rendered, "ctrl+t") {
		t.Fatalf("expected fold indicator in long exec output, got: %s", rendered)
	}
	raw := strings.Join(cell.RawLines(), "\n")
	if !strings.Contains(raw, "line 1") || !strings.Contains(raw, "line 8") {
		t.Fatalf("raw lines should not be folded, got: %s", raw)
	}
}

func TestErrorCellDiagnosticRenderingIsInline(t *testing.T) {
	cell := &ErrorCell{ErrorKind: ErrorKindModelNotFound, Title: "Model Not Supported", Badge: "MODEL_NOT_FOUND", Text: "Model not supported detail", Suggestions: []string{"Did you mean: fallback-model"}}
	rendered := ansi.Strip(strings.Join(cell.RenderWidth(80), "\n"))
	if rendered != "× Model Not Supported · MODEL_NOT_FOUND" {
		t.Fatalf("inline diagnostic = %q", rendered)
	}
	if strings.Contains(rendered, "Did you mean") || strings.Contains(rendered, "detail") {
		t.Fatalf("inline diagnostic leaked verbose detail: %q", rendered)
	}
	raw := strings.Join(cell.RawLines(), "\n")
	if !strings.Contains(raw, "Model not supported detail") || !strings.Contains(raw, "Did you mean: fallback-model") {
		t.Fatalf("raw diagnostic lost details: %q", raw)
	}
}

func TestErrorCellIncompleteStreamUsesDistinctStableCode(t *testing.T) {
	cell := &ErrorCell{ErrorKind: ErrorKindStreamIncomplete, Title: "Provider Stream Ended Early", Badge: "STREAM_INCOMPLETE", Text: "provider closed before terminal event"}
	rendered := ansi.Strip(strings.Join(cell.RenderWidth(80), "\n"))
	if rendered != "× Provider Stream Ended Early · STREAM_INCOMPLETE" {
		t.Fatalf("incomplete stream diagnostic = %q", rendered)
	}
	if strings.Contains(rendered, "STREAM_TIMEOUT") {
		t.Fatalf("incomplete stream rendered as timeout: %q", rendered)
	}
}

func TestErrorCellServerOverloadedUsesStableCode(t *testing.T) {
	cell := &ErrorCell{ErrorKind: ErrorKindServerOverloaded, Title: "Provider Server Overloaded", Badge: "503 SERVER_ERROR", Text: "upstream unavailable"}
	rendered := ansi.Strip(strings.Join(cell.RenderWidth(80), "\n"))
	if rendered != "× Provider Server Overloaded · PROVIDER_OVERLOADED" {
		t.Fatalf("server overload diagnostic = %q", rendered)
	}
	if strings.Contains(rendered, "503") || strings.Contains(rendered, "SERVER_ERROR") {
		t.Fatalf("stable diagnostic leaked provider transport code: %q", rendered)
	}
}

func TestErrorCellFallbackRendering(t *testing.T) {
	cell := &ErrorCell{Title: "read", Text: "file not found"}
	rendered := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(rendered, "read: file not found") {
		t.Fatalf("expected simple fallback error line, got:\n%s", rendered)
	}
	raw := strings.Join(cell.RawLines(), "\n")
	if raw != "read: file not found" {
		t.Fatalf("expected raw text to match, got %q", raw)
	}
}

func TestToolCellReadDetailFollowsDensity(t *testing.T) {
	minimal := &ToolCell{Name: "read", Target: "internal/tui/theme.go", ToolKind: tool.KindRead, Body: "// Package tui\npackage tui\n\nimport \"fmt\"\n", Summary: "4 lines (45 B)"}
	if rendered := strings.Join(minimal.RenderWidth(80), "\n"); strings.Contains(rendered, "↳") {
		t.Fatalf("minimal read leaked excerpt:\n%s", rendered)
	}
	detailed := *minimal
	detailed.ShowDetail = true
	if rendered := strings.Join(detailed.RenderWidth(80), "\n"); !strings.Contains(rendered, "package tui") || !strings.Contains(rendered, "↳") {
		t.Fatalf("detailed read missing excerpt:\n%s", rendered)
	}
}

func TestToolFailureSuggestions(t *testing.T) {
	notFoundSugg := transcriptutil.ToolFailureSuggestions("read", tool.ErrorCodeNotFound)
	if len(notFoundSugg) == 0 {
		t.Fatalf("expected suggestions for read not found error")
	}
	protectedSugg := transcriptutil.ToolFailureSuggestions("read", tool.ErrorCodeProtectedPath)
	if len(protectedSugg) == 0 || !strings.Contains(protectedSugg[0], "workspace protection rules") {
		t.Fatalf("expected suggestions for protected path error")
	}
	escapeSugg := transcriptutil.ToolFailureSuggestions("read", tool.ErrorCodeOutsideWorkspace)
	if len(escapeSugg) != 1 || escapeSugg[0] != "use . or a workspace-relative path" {
		t.Fatalf("expected actionable suggestions for outside workspace error: %#v", escapeSugg)
	}
}

func TestHistoryStateAlternateRenderCacheInvalidatesOnCommit(t *testing.T) {
	state := NewHistoryState(1000)
	state.Append(&AssistantCell{Text: "first"})
	before := strings.Join(state.RenderLinesAt(40), "\n")
	if !strings.Contains(before, "first") {
		t.Fatalf("initial alternate render missing content: %q", before)
	}
	state.Append(&AssistantCell{Text: "second"})
	after := strings.Join(state.RenderLinesAt(40), "\n")
	if !strings.Contains(after, "second") {
		t.Fatalf("alternate render cache was stale after commit: %q", after)
	}
}

func TestHistoryStateAlternateRenderCacheTracksWidth(t *testing.T) {
	state := NewHistoryState(1000)
	state.Append(&AssistantCell{Text: "a long assistant response that wraps differently by width"})
	wide := state.RenderLinesAt(60)
	narrow := state.RenderLinesAt(20)
	if len(narrow) <= len(wide) {
		t.Fatalf("narrow alternate render lines = %d, want more than wide %d", len(narrow), len(wide))
	}
}

func TestHistoryStateRenderContentMatchesRenderLines(t *testing.T) {
	state := NewHistoryState(50000)
	state.Append(&UserCell{Text: "question"})
	state.Append(&AssistantCell{Text: "## answer\n\n- one\n- two"})
	state.AppendAssistantDelta("streaming **tail**")
	want := strings.Join(state.RenderLines(), "\n")
	if got := state.RenderContent(); got != want {
		t.Fatalf("RenderContent mismatch\nwant: %q\n got: %q", want, got)
	}
	state.CommitActive()
	want = strings.Join(state.RenderLines(), "\n")
	if got := state.RenderContent(); got != want {
		t.Fatalf("committed RenderContent mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestHistoryStateRenderTailContentMatchesFullSuffix(t *testing.T) {
	state := NewHistoryState(50000)
	for i := 0; i < 8; i++ {
		state.Append(&AssistantCell{Text: fmt.Sprintf("answer %d\nsecond line", i)})
	}
	state.AppendAssistantDelta("stream one\nstream two\nstream three")
	full := strings.Split(state.RenderContent(), "\n")
	got, truncated := state.RenderTailContent(5)
	if !truncated {
		t.Fatal("expected tail render to truncate older content")
	}
	want := strings.Join(full[len(full)-5:], "\n")
	if got != want {
		t.Fatalf("tail mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestHistoryStateRenderTailContentMatchesFullSuffixInsideOpenFence(t *testing.T) {
	state := NewHistoryState(50000)
	for i := 0; i < 12; i++ {
		state.Append(&AssistantCell{Text: fmt.Sprintf("answer %d\nsecond line", i)})
	}
	state.AppendAssistantDelta("```go\npackage main\nfunc main() {\nprintln(\"streaming\")")
	full := strings.Split(state.RenderContent(), "\n")
	got, truncated := state.RenderTailContent(6)
	if !truncated {
		t.Fatal("expected fenced tail render to truncate older content")
	}
	want := strings.Join(full[len(full)-6:], "\n")
	if got != want {
		t.Fatalf("fenced tail mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestExecCellSeparatesStderrAndStreamTruncation(t *testing.T) {
	exit1 := 1
	cell := &ExecCell{Command: "go test ./...", Stdout: "package a ok\n", Stderr: "package b failed\n", ExitCode: &exit1, StdoutTruncated: true, Truncated: true, FailureCode: tool.ErrorCodeCommandFailed}
	rendered := strings.Join(cell.RenderWidth(80), "\n")
	for _, want := range []string{"Go test ./...", "exit 1", "package a ok", "stderr:", "package b failed", "stdout truncated"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("render missing %q:\n%s", want, rendered)
		}
	}
	raw := strings.Join(cell.RawLines(), "\n")
	for _, want := range []string{"stderr:", "exit 1"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("raw missing %q:\n%s", want, raw)
		}
	}
	if strings.Contains(raw, "failure: command_failed") {
		t.Fatalf("raw redundantly exposes command_failed next to exit code:\n%s", raw)
	}
}

func TestExecCellHugeMixedOutputStaysBoundedButRawRemainsComplete(t *testing.T) {
	exit := 1
	stdout := strings.Repeat("stdout payload ไทย 東京 "+strings.Repeat("x", 80)+"\n", 2000)
	stderr := strings.Repeat("stderr payload "+strings.Repeat("y", 80)+"\n", 1200)
	cell := &ExecCell{Name: "bash", Command: "stress-output", Stdout: stdout, Stderr: stderr, ExitCode: &exit}
	rendered := cell.RenderWidth(40)
	if len(rendered) > 16 {
		t.Fatalf("huge mixed output rendered %d viewport lines, want bounded presentation", len(rendered))
	}
	joined := testPlain(strings.Join(rendered, "\n"))
	for _, want := range []string{"more · ctrl+t", "stderr:", "exit 1"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("bounded mixed output missing %q: %q", want, joined)
		}
	}
	for _, line := range rendered {
		if got := ansi.StringWidth(line); got > 40 {
			t.Fatalf("huge mixed output line width=%d exceeds 40: %q", got, line)
		}
	}
	raw := strings.Join(cell.RawLines(), "\n")
	if !strings.Contains(raw, "stdout payload ไทย 東京") || !strings.Contains(raw, "stderr payload") {
		t.Fatal("raw transcript lost huge stdout/stderr content")
	}
}

func TestAgentToolCellRendersOrchestrationSemantics(t *testing.T) {
	running := (&AgentToolCell{Name: "subagent", Target: "explorer-7", Running: true, Spinner: "⠋"}).RenderWidth(80)
	if got := strings.Join(running, "\n"); !strings.Contains(got, "Coordinating subagents") || strings.Contains(got, "wait agent") {
		t.Fatalf("running agent cell=%q", got)
	}
	completed := (&AgentToolCell{Name: "subagent", Target: "explorer-7", Summary: "explorer-7 · completed · found routing issue"}).RenderWidth(80)
	if got := strings.Join(completed, "\n"); !strings.Contains(got, "found routing issue") {
		t.Fatalf("completed agent cell=%q", got)
	}
}

func TestApplyTurnEventsCoalescesContiguousText(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.applyTurnEvents([]turn.Event{{Kind: turn.EventTextDelta, Round: 2, Text: "alpha"}, {Kind: turn.EventTextDelta, Round: 2, Text: " beta"}, {Kind: turn.EventTextDelta, Round: 2, Text: " gamma"}})
	active, ok := m.historyState.Active().(*AssistantCell)
	if !ok {
		t.Fatalf("active cell=%T, want assistant", m.historyState.Active())
	}
	if active.Text != "alpha beta gamma" {
		t.Fatalf("assistant text=%q", active.Text)
	}
	if m.turnProgress.Round != 2 {
		t.Fatalf("round=%d, want 2", m.turnProgress.Round)
	}
}

func TestAgentToolCellRawLinesUseOrchestrationLabel(t *testing.T) {
	cell := AgentToolCell{Name: "subagent", Target: "explorer-7", Running: true, Spinner: "⠋"}
	got := strings.Join(cell.RawLines(), "\n")
	if got != "Coordinating subagents" {
		t.Fatalf("RawLines()=%q", got)
	}
}

func TestPatchCellRenderingPolish(t *testing.T) {
	patch := &PatchCell{Name: "edit", Summary: "1 file", Paths: []string{"cmd/protonman/main.go"}, Body: "Wrote file successfully to cmd/protonman/main.go."}
	rendered := patch.RenderWidth(80)
	joined := testPlain(strings.Join(rendered, "\n"))
	if strings.Contains(joined, "✓ +") {
		t.Fatalf("unexpected glyph stutter '✓ +' in patch cell header:\n%s", joined)
	}
	if !strings.Contains(joined, "✓") || !strings.Contains(joined, "Edit") {
		t.Fatalf("expected clean checkmark and tool display name 'Edit' in patch cell header:\n%s", joined)
	}
	if strings.Contains(joined, "Wrote file successfully to") {
		t.Fatalf("expected redundant body to be suppressed in patch cell:\n%s", joined)
	}
	multiPatch := &PatchCell{Name: "edit", Summary: "6 files", Paths: []string{"file1.go", "file2.go", "file3.go", "file4.go", "file5.go", "file6.go"}}
	multiRendered := multiPatch.RenderWidth(80)
	multiJoined := strings.Join(multiRendered, "\n")
	if !strings.Contains(multiJoined, "file1.go") || !strings.Contains(multiJoined, "file3.go") {
		t.Fatalf("expected first 3 files to be visible:\n%s", multiJoined)
	}
	if strings.Contains(multiJoined, "file4.go") {
		t.Fatalf("expected file4.go and beyond to be folded:\n%s", multiJoined)
	}
	if !strings.Contains(multiJoined, "+3 more files") {
		t.Fatalf("expected fold hint for remaining files:\n%s", multiJoined)
	}
}

func TestExecCellClampsLongLinesAndHighlightsDiff(t *testing.T) {
	longLine := "data: " + strings.Repeat("x", 200)
	cell := &ExecCell{Name: "bash", Command: "curl https://api.example.com", Stdout: longLine}
	rendered := cell.RenderWidth(60)
	joined := testPlain(strings.Join(rendered, "\n"))
	if strings.Contains(joined, strings.Repeat("x", 200)) {
		t.Fatalf("expected 200-char line to be clamped horizontally in viewport:\n%s", joined)
	}
	if !strings.Contains(joined, "…") {
		t.Fatalf("expected ellipsis truncation indicator on long line:\n%s", joined)
	}
	diffCell := &ExecCell{Name: "bash", Command: "git diff", Stdout: "@@ -1,3 +1,4 @@\n+func New() {}\n-old()\n"}
	diffRendered := diffCell.RenderWidth(80)
	diffJoined := strings.Join(diffRendered, "\n")
	if !strings.Contains(diffJoined, "+func New() {}") || !strings.Contains(diffJoined, "-old()") {
		t.Fatalf("expected diff lines to be preserved in diff render:\n%s", diffJoined)
	}
}

func TestActivateSkillFallbackToTarget(t *testing.T) {
	cell := &ToolCell{Name: "skill", Target: `"pdf-processing"`, Body: "Loaded skill instructions successfully.", ToolKind: tool.KindRead}
	rendered := cell.RenderWidth(80)
	joined := testPlain(strings.Join(rendered, "\n"))
	if !strings.Contains(joined, `"pdf-processing"`) {
		t.Fatalf("expected target skill name to appear in header:\n%s", joined)
	}
	if !strings.Contains(joined, "Activated skill") {
		t.Fatalf("expected 'Activated skill' in header:\n%s", joined)
	}
}

func TestTaskBodySuppressionInTranscript(t *testing.T) {
	if !shouldSuppressBody(tool.KindTask, "todo") {
		t.Fatal("expected KindTask to suppress body in transcript")
	}
	if !shouldSuppressBody(tool.KindTask, "todo") {
		t.Fatal("expected todo to suppress body in transcript")
	}
	if !shouldSuppressBody(tool.KindEdit, "edit") {
		t.Fatal("expected KindEdit to suppress body in transcript")
	}
}

func TestEditToolUsesStructuredPatchCell(t *testing.T) {
	registry := newNamedTestRegistry(tool.Definition{Name: "edit", Description: "apply a workspace patch", Kind: tool.KindEdit, PermissionDetailKey: "patch"})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	m := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "")
	call, err := tool.NewCall("edit-1", "edit", []byte(`{"action":"patch","patch":"*** Begin Patch\n*** Update File: internal/a.go\n*** End Patch"}`))
	if err != nil {
		t.Fatalf("NewCall() error = %v", err)
	}
	m.appendToolCall(call)
	cell, ok := m.historyState.Active().(*PatchCell)
	if !ok {
		t.Fatalf("active cell = %T, want *PatchCell", m.historyState.Active())
	}
	if len(cell.Paths) != 1 || cell.Paths[0] != "internal/a.go" {
		t.Fatalf("patch paths = %#v, want internal/a.go", cell.Paths)
	}
}

func TestTranscriptRawRichTogglePreservesRelativeScrollPosition(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.showWelcome = false
	for i := 0; i < 80; i++ {
		m.appendUser(fmt.Sprintf("question %02d", i))
		m.appendAssistant("answer with **markdown** and some detail")
	}
	m.panes.showTranscript = true
	m.refreshTranscriptViewport(true)
	m.panes.transcript.SetYOffset(m.panes.transcript.YOffset() / 2)
	before := m.panes.transcript.ScrollPercent()
	if before <= 0 || before >= 1 {
		t.Fatalf("test setup scroll percent=%f, want middle position", before)
	}
	_ = m.updateTranscriptKey(testText("r"))
	afterRaw := m.panes.transcript.ScrollPercent()
	if diff := math.Abs(afterRaw - before); diff > 0.08 {
		t.Fatalf("raw toggle scroll percent jumped from %.3f to %.3f", before, afterRaw)
	}
	_ = m.updateTranscriptKey(testText("r"))
	afterRich := m.panes.transcript.ScrollPercent()
	if diff := math.Abs(afterRich - before); diff > 0.08 {
		t.Fatalf("rich toggle scroll percent jumped from %.3f to %.3f", before, afterRich)
	}
}

func TestTranscriptOverlayIncludesLiveAssistantTail(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.resize(80, 24)
	m.appendAssistantDelta("streaming now")
	m.panes.showTranscript = true
	m.refreshTranscriptViewport(true)
	if !strings.Contains(m.transcriptOverlayView(), "streaming now") {
		t.Fatalf("transcript overlay omitted active cell: %s", m.transcriptOverlayView())
	}
	_ = m.updateTranscriptKey(testText("r"))
	if !m.panes.rawTranscript {
		t.Fatal("r did not toggle raw transcript mode")
	}
}

func TestInitialMessagesRestoreIntoHistoryAndNextTurn(t *testing.T) {
	registry, _ := newBubbleTestRegistry()
	service := newBubbleTestService(t, registry, permission.ModeAsk, permission.Config{})
	messages := []model.Message{{Role: model.RoleUser, Content: "previous question"}, {Role: model.RoleAssistant, Content: "previous answer"}}
	m := newBubbleModel(context.Background(), service, registry, emptyTodoItems(), nil, newPermissionBridge(), "", messages)
	plain := plainTranscript(m)
	if !strings.Contains(plain, "previous question") || !strings.Contains(plain, "previous answer") {
		t.Fatalf("restored transcript = %q", plain)
	}
	if len(m.messages) != 2 {
		t.Fatalf("provider history length = %d, want 2", len(m.messages))
	}
}
