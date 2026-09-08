package tui

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/engine/turn"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/tool"
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
	state.StartTool("read_file")
	state.StartTool("grep")

	state.CompleteTool(ToolCell{Name: "read_file", Body: "contents"})
	cells := state.Cells()
	if len(cells) != 2 {
		t.Fatalf("cell count = %d, want 2", len(cells))
	}
	first, ok := cells[0].(*ToolCell)
	if !ok || first.Running || first.Body != "contents" {
		t.Fatalf("first tool = %#v, want completed read_file", cells[0])
	}
	second, ok := cells[1].(*ToolCell)
	if !ok || !second.Running || second.Name != "grep" {
		t.Fatalf("second tool = %#v, want active grep", cells[1])
	}
}

func TestHistoryStateUsesCallIDForSameNameParallelTools(t *testing.T) {
	state := NewHistoryState(100)
	state.StartToolCall("read-1", "read_file")
	state.StartToolCall("read-2", "read_file")

	state.CompleteTool(ToolCell{CallID: "read-1", Name: "read_file", Body: "first"})
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

	state.CompleteTool(ToolCell{CallID: "read-2", Name: "read_file", Body: "second"})
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
	}{
		{HistoryCellUnknown, "unknown"},
		{HistoryCellUser, "user"},
		{HistoryCellAssistant, "assistant"},
		{HistoryCellTool, "tool"},
		{HistoryCellSystem, "system"},
		{HistoryCellError, "error"},
		{HistoryCellKind(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("HistoryCellKind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

func TestHistoryStateRunningToolSpinner(t *testing.T) {
	state := NewHistoryState(100)
	state.SetSpinnerFrame("⠋")

	state.StartTool("read_file")
	lines := state.RenderLines()
	if len(lines) == 0 || !strings.Contains(lines[len(lines)-1], "⠋") {
		t.Fatalf("expected running tool to contain spinner frame ⠋, got: %v", lines)
	}

	state.SetSpinnerFrame("⠙")
	lines = state.RenderLines()
	if len(lines) == 0 || !strings.Contains(lines[len(lines)-1], "⠙") {
		t.Fatalf("expected running tool to contain updated spinner frame ⠙, got: %v", lines)
	}

	state.CompleteTool(ToolCell{Name: "read_file", Body: "done"})
	lines = state.RenderLines()
	if len(lines) == 0 || strings.Contains(lines[0], "⠙") || strings.Contains(lines[0], "…") {
		t.Fatalf("completed tool should not contain spinner, got: %v", lines)
	}
}

func TestHistoryStateThinkingCellLifecycle(t *testing.T) {
	t.Run("converts to assistant on first delta", func(t *testing.T) {
		state := NewHistoryState(100)
		state.SetSpinnerFrame("⠋")
		state.StartThinking()

		active := state.Active()
		if _, ok := active.(*ThinkingCell); !ok {
			t.Fatalf("active cell = %T, want *ThinkingCell", active)
		}
		rendered := state.RenderLines()
		if len(rendered) == 0 || !strings.Contains(rendered[0], "Thinking…") {
			t.Fatalf("expected thinking render, got: %v", rendered)
		}

		state.AppendAssistantDelta("Hello world")
		active = state.Active()
		assistant, ok := active.(*AssistantCell)
		if !ok {
			t.Fatalf("active cell = %T, want *AssistantCell", active)
		}
		if assistant.Text != "Hello world" {
			t.Fatalf("assistant text = %q, want Hello world", assistant.Text)
		}
	})

	t.Run("discarded on commit if no text", func(t *testing.T) {
		state := NewHistoryState(100)
		state.StartThinking()
		state.CommitActive()
		if state.Active() != nil {
			t.Fatalf("active cell = %T after commit, want nil", state.Active())
		}
		if len(state.Cells()) != 0 {
			t.Fatalf("cells length = %d, want 0 (thinking cell should not be committed)", len(state.Cells()))
		}
	})
}

func TestActivateSkillToolCellCompactRendering(t *testing.T) {
	xmlBody := `<skill_content name="golang-performance">
**Persona:** You are a Go performance engineer.
# Go Performance Optimization
1. Profile before optimizing...
</skill_content>`

	cell := ToolCell{
		Name: "activate_skill",
		Body: xmlBody,
	}

	raw := cell.RawLines()
	for _, line := range raw {
		if strings.Contains(line, "Profile before optimizing") {
			t.Fatalf("RawLines should not contain raw instruction markdown, got: %v", raw)
		}
	}

	rendered := cell.Render()
	joined := strings.Join(rendered, "\n")
	if !strings.Contains(joined, `Activated skill "golang-performance"`) {
		t.Fatalf("expected compact activation badge in render, got: %s", joined)
	}
	if strings.Contains(joined, "Profile before optimizing") {
		t.Fatalf("rendered output contains full skill instructions: %s", joined)
	}
}

func TestLoadInitialMessagesCompactsSkillDetail(t *testing.T) {
	bm := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "")
	bm.loadInitialMessages([]model.Message{
		{
			Role:     model.RoleTool,
			ToolName: "activate_skill",
			Content:  `<skill_content name="golang-code-style">\n# Full instructions...\n</skill_content>`,
		},
		{
			Role:    model.RoleUser,
			Content: "Activated skill pdf-tool [user]:\n# PDF Guide\nLong content here...",
		},
	})

	rendered := strings.Join(bm.historyState.RenderLines(), "\n")
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
	state.SetSpinnerFrame("⠋")

	// Running state
	runningCell := &ToolCell{
		CallID:   "call-web-1",
		Name:     "web_fetch",
		Target:   "https://protonman.dev",
		ToolKind: tool.KindWebFetch,
		Running:  true,
	}
	state.StartToolCell(runningCell)

	rendered := state.RenderLines()
	joinedRunning := strings.Join(rendered, "\n")
	if !strings.Contains(joinedRunning, "↗") || !strings.Contains(joinedRunning, "Fetch") || !strings.Contains(joinedRunning, "https://protonman.dev") {
		t.Fatalf("expected running cell to show category icon and target, got: %s", joinedRunning)
	}
	if strings.Contains(joinedRunning, "web_fetch") {
		t.Fatalf("raw 'web_fetch' should not appear in rendered output: %s", joinedRunning)
	}

	// Completed state
	completedCell := &ToolCell{
		CallID:   "call-web-1",
		Name:     "web_fetch",
		Target:   "https://protonman.dev",
		ToolKind: tool.KindWebFetch,
		Body:     htmlPayload,
		Summary:  summarizeToolOutput("web_fetch", tool.KindWebFetch, "https://protonman.dev", htmlPayload, nil, false),
	}
	state.CompleteToolCall("call-web-1", "web_fetch", completedCell)

	rendered = state.RenderLines()
	joinedCompleted := strings.Join(rendered, "\n")

	// 1. Must contain success glyph, tool name, target, and parsed page title
	if !strings.Contains(joinedCompleted, "✓") || !strings.Contains(joinedCompleted, "https://protonman.dev") {
		t.Fatalf("expected completed summary header, got: %s", joinedCompleted)
	}
	if !strings.Contains(joinedCompleted, "รวม AI ราคาถูกใน API เดียว | protonmanAI") {
		t.Fatalf("expected page title in summary, got: %s", joinedCompleted)
	}

	// 2. Must SUPPRESS raw HTML tags from the chat viewport
	if strings.Contains(joinedCompleted, "<!DOCTYPE html>") || strings.Contains(joinedCompleted, "<body>") {
		t.Fatalf("raw HTML was not suppressed from rendered viewport lines: %s", joinedCompleted)
	}

	// 3. Must PRESERVE raw HTML in raw transcript for ctrl+t and exports
	raw := state.Raw()
	if !strings.Contains(raw, "<!DOCTYPE html>") || !strings.Contains(raw, "Protonman") {
		t.Fatalf("raw transcript failed to preserve complete HTML payload: %s", raw)
	}
}

func TestToolCellRefinedRenderingReadFile(t *testing.T) {
	fileContent := strings.Repeat("fmt.Println(\"code\")\n", 50)
	cell := &ToolCell{
		Name:     "read_file",
		Target:   "internal/tui/theme.go",
		ToolKind: tool.KindRead,
		Body:     fileContent,
		Summary:  summarizeToolOutput("read_file", tool.KindRead, "internal/tui/theme.go", fileContent, nil, false),
	}

	rendered := strings.Join(cell.Render(), "\n")
	if !strings.Contains(rendered, "50 lines") || !strings.Contains(rendered, "internal/tui/theme.go") {
		t.Fatalf("expected summary with line count and target, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Read") {
		t.Fatalf("expected action verb 'Read' in header, got: %s", rendered)
	}
	if strings.Contains(rendered, "read_file") {
		t.Fatalf("raw 'read_file' should be replaced by SSOT DisplayName, got: %s", rendered)
	}
	// Raw code should not flood the rendered viewport
	if strings.Contains(rendered, "fmt.Println") {
		t.Fatalf("raw file contents should be suppressed from viewport, got: %s", rendered)
	}

	// Raw lines should retain full content
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
	cell := &ExecCell{
		Command:  "npm test",
		Body:     longOutput,
		ExitCode: &exit0,
	}

	rendered := strings.Join(cell.Render(), "\n")
	if !strings.Contains(rendered, "Npm test") || strings.Contains(rendered, "exit 0") {
		t.Fatalf("expected semantic command title without redundant exit 0, got: %s", rendered)
	}
	if !strings.Contains(rendered, "lines hidden") || !strings.Contains(rendered, "ctrl+t") {
		t.Fatalf("expected fold indicator in long exec output, got: %s", rendered)
	}

	raw := strings.Join(cell.RawLines(), "\n")
	if !strings.Contains(raw, "line 1") || !strings.Contains(raw, "line 8") {
		t.Fatalf("raw lines should not be folded, got: %s", raw)
	}
}

func TestErrorCellCardRendering(t *testing.T) {
	cell := &ErrorCell{
		ErrorKind:   ErrorKindModelNotFound,
		Title:       "Model Not Supported",
		Badge:       "MODEL_NOT_FOUND",
		Text:        "Model 'gpt-nonexistent' is not supported by provider 'opencode'.",
		Suggestions: []string{"Did you mean: nemotron-3.5-lightning-free", "Run /provider to configure an available model"},
	}

	rendered := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(rendered, "MODEL_NOT_FOUND") {
		t.Fatalf("expected rendered card to contain badge, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Model Not Supported") {
		t.Fatalf("expected rendered card to contain title, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Suggestions:") {
		t.Fatalf("expected rendered card to contain Suggestions header, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Did you mean: nemotron-3.5-lightning-free") {
		t.Fatalf("expected rendered card to contain model suggestions, got:\n%s", rendered)
	}

	raw := strings.Join(cell.RawLines(), "\n")
	if !strings.Contains(raw, "[MODEL_NOT_FOUND] Model Not Supported") {
		t.Fatalf("raw lines missing formatted header, got:\n%s", raw)
	}
}

func TestErrorCellFallbackRendering(t *testing.T) {
	cell := &ErrorCell{
		Title: "read_file",
		Text:  "file not found",
	}

	rendered := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(rendered, "read_file: file not found") {
		t.Fatalf("expected simple fallback error line, got:\n%s", rendered)
	}

	raw := strings.Join(cell.RawLines(), "\n")
	if raw != "read_file: file not found" {
		t.Fatalf("expected raw text to match, got %q", raw)
	}
}

func TestToolCellRenderReadFileExcerpt(t *testing.T) {
	cell := &ToolCell{
		Name:     "read_file",
		Target:   "internal/tui/theme.go",
		ToolKind: tool.KindRead,
		Body:     "// Package tui\npackage tui\n\nimport \"fmt\"\n",
		Summary:  "4 lines (45 B)",
	}

	rendered := strings.Join(cell.RenderWidth(80), "\n")
	if !strings.Contains(rendered, "package tui") || !strings.Contains(rendered, "↳") {
		t.Fatalf("expected rendered cell to contain excerpt '↳ package tui', got:\n%s", rendered)
	}
}

func TestToolFailureSuggestions(t *testing.T) {
	notFoundSugg := toolFailureSuggestions("read_file", tool.ErrorCodeNotFound)
	if len(notFoundSugg) == 0 {
		t.Fatalf("expected suggestions for read_file not found error")
	}

	protectedSugg := toolFailureSuggestions("read_file", tool.ErrorCodeProtectedPath)
	if len(protectedSugg) == 0 || !strings.Contains(protectedSugg[0], "workspace protection rules") {
		t.Fatalf("expected suggestions for protected path error")
	}

	escapeSugg := toolFailureSuggestions("read_file", tool.ErrorCodeOutsideWorkspace)
	if len(escapeSugg) == 0 || !strings.Contains(escapeSugg[0], "workspace root") {
		t.Fatalf("expected suggestions for outside workspace error")
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

func TestAssistantIncrementalMarkdownMatchesFullRenderer(t *testing.T) {
	chunks := []string{
		"# Heading\n",
		"\nParagraph with **bold",
		" text** and `code`.\n",
		"- first item\n- second item\n",
		"> quote\n",
		"```go\n",
		"fmt.Println(\"hello\")\n",
		"```\n",
		"[link](https://example.com)",
	}
	cell := &AssistantCell{}
	var text string
	for i, chunk := range chunks {
		text += chunk
		cell.Text = text
		trimmed := strings.TrimRight(text, "\n")
		got := cell.renderMarkdownIncremental(trimmed, 48)
		want := renderMarkdownLines(trimmed, 48)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("chunk %d incremental render mismatch\ngot:  %#v\nwant: %#v", i, got, want)
		}
	}
}

func TestAssistantStreamingBufferRecoversFromDirectTextReplacement(t *testing.T) {
	state := NewHistoryState(1000)
	state.AppendAssistantDelta("hello")
	assistant := state.Active().(*AssistantCell)
	assistant.Text = "replacement"
	state.AppendAssistantDelta(" tail")
	if assistant.Text != "replacement tail" {
		t.Fatalf("assistant text = %q, want replacement tail", assistant.Text)
	}
}

func TestCommitActiveSealsAssistantStreamingBuffer(t *testing.T) {
	state := NewHistoryState(1000)
	state.AppendAssistantDelta("hello")
	state.AppendAssistantDelta(" world")
	assistant := state.Active().(*AssistantCell)
	if !assistant.streamActive {
		t.Fatal("assistant stream buffer was not active before commit")
	}
	state.CommitActive()
	if assistant.Text != "hello world" {
		t.Fatalf("assistant text = %q, want hello world", assistant.Text)
	}
	if assistant.streamActive || assistant.streamBuilder.Len() != 0 {
		t.Fatal("assistant stream buffer remained active after commit")
	}
}

func TestAssistantIncrementalMarkdownPreservesTrailingNewlineSemantics(t *testing.T) {
	for _, text := range []string{
		"line",
		"line\n",
		"line\n\n",
		"line\n\n\n",
		"```go\nfmt.Println(1)\n",
	} {
		cell := &AssistantCell{Text: text}
		got := cell.RenderWidth(48)
		trimmed := strings.TrimRight(text, "\n")
		wantMarkdown := renderMarkdownLines(trimmed, 46)
		want := decorateAssistantLines(wantMarkdown, 0)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("trailing newline render mismatch for %q\ngot:  %#v\nwant: %#v", text, got, want)
		}
	}
}

func TestAssistantIncrementalMarkdownResetsForWidthAndMutation(t *testing.T) {
	cell := &AssistantCell{Text: "first line\nsecond line with **bold**"}
	_ = cell.RenderWidth(60)
	cell.Text += "\nthird line"
	wide := cell.RenderWidth(60)
	cell.Text = "replacement text\nwith a different prefix"
	narrow := cell.RenderWidth(24)
	if len(wide) == 0 || len(narrow) == 0 {
		t.Fatal("incremental assistant renderer returned no lines")
	}
	joined := strings.Join(narrow, "\n")
	if strings.Contains(joined, "first line") || !strings.Contains(joined, "replacement") {
		t.Fatalf("incremental cache survived replacement: %q", joined)
	}
}

func TestHistoryStateSpinnerFrameReportsVisualChanges(t *testing.T) {
	state := NewHistoryState(1000)
	state.AppendAssistantDelta("streaming")
	if state.SetSpinnerFrame("a") {
		t.Fatal("assistant cell reported a visual spinner change")
	}
	state.CommitActive()
	state.StartThinking()
	if !state.SetSpinnerFrame("b") {
		t.Fatal("thinking cell did not report spinner change")
	}
	state.StartTool("read_file")
	if !state.SetSpinnerFrame("c") {
		t.Fatal("running tool did not report spinner change")
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

func TestExecCellSeparatesStderrAndStreamTruncation(t *testing.T) {
	exit1 := 1
	cell := &ExecCell{
		Command: "go test ./...", Stdout: "package a ok\n", Stderr: "package b failed\n",
		ExitCode: &exit1, StdoutTruncated: true, Truncated: true,
		FailureCode: tool.ErrorCodeCommandFailed,
	}
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

func TestAgentToolCellRendersOrchestrationSemantics(t *testing.T) {
	running := (&AgentToolCell{Name: "wait_agent", Target: "explorer-7", Running: true, Spinner: "⠋"}).RenderWidth(80)
	if got := strings.Join(running, "\n"); !strings.Contains(got, "Waiting for explorer-7") || strings.Contains(got, "wait_agent") {
		t.Fatalf("running agent cell=%q", got)
	}
	completed := (&AgentToolCell{Name: "wait_agent", Target: "explorer-7", Summary: "explorer-7 · completed · found routing issue"}).RenderWidth(80)
	if got := strings.Join(completed, "\n"); !strings.Contains(got, "found routing issue") {
		t.Fatalf("completed agent cell=%q", got)
	}
}

func TestApplyTurnEventsCoalescesContiguousText(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, nil)
	m.applyTurnEvents([]turn.Event{
		{Kind: turn.EventTextDelta, Round: 2, Text: "alpha"},
		{Kind: turn.EventTextDelta, Round: 2, Text: " beta"},
		{Kind: turn.EventTextDelta, Round: 2, Text: " gamma"},
	})
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
	cell := AgentToolCell{Name: "wait_agent", Target: "explorer-7", Running: true, Spinner: "⠋"}
	got := strings.Join(cell.RawLines(), "\n")
	if got != "Waiting for explorer-7" {
		t.Fatalf("RawLines()=%q", got)
	}
}

func TestPatchCellRenderingPolish(t *testing.T) {
	// 1. Verify no "✓ + " glyph stutter
	patch := &PatchCell{
		Name:    "write_file",
		Summary: "1 file",
		Paths:   []string{"cmd/protonman/main.go"},
		Body:    "Wrote file successfully to cmd/protonman/main.go.",
	}
	rendered := patch.RenderWidth(80)
	joined := strings.Join(rendered, "\n")
	if strings.Contains(joined, "✓ +") {
		t.Fatalf("unexpected glyph stutter '✓ +' in patch cell header:\n%s", joined)
	}
	if !strings.Contains(joined, "✓") || !strings.Contains(joined, "Write") {
		t.Fatalf("expected clean checkmark and tool display name 'Write' in patch cell header:\n%s", joined)
	}
	if strings.Contains(joined, "write_file") {
		t.Fatalf("expected raw tool name 'write_file' to NOT appear in patch cell header:\n%s", joined)
	}
	// Redundant body should be suppressed when paths are present
	if strings.Contains(joined, "Wrote file successfully to") {
		t.Fatalf("expected redundant body to be suppressed in patch cell:\n%s", joined)
	}

	// 2. Multi-file patch capping
	multiPatch := &PatchCell{
		Name:    "apply_patch",
		Summary: "6 files",
		Paths: []string{
			"file1.go",
			"file2.go",
			"file3.go",
			"file4.go",
			"file5.go",
			"file6.go",
		},
	}
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
	// 1. Clamp long line
	longLine := "data: " + strings.Repeat("x", 200)
	cell := &ExecCell{
		Name:    "bash",
		Command: "curl https://api.example.com",
		Stdout:  longLine,
	}
	rendered := cell.RenderWidth(60)
	joined := strings.Join(rendered, "\n")
	if strings.Contains(joined, strings.Repeat("x", 200)) {
		t.Fatalf("expected 200-char line to be clamped horizontally in viewport:\n%s", joined)
	}
	if !strings.Contains(joined, "…") {
		t.Fatalf("expected ellipsis truncation indicator on long line:\n%s", joined)
	}

	// 2. Diff syntax highlighting
	diffCell := &ExecCell{
		Name:    "bash",
		Command: "git diff",
		Stdout:  "@@ -1,3 +1,4 @@\n+func New() {}\n-old()\n",
	}
	diffRendered := diffCell.RenderWidth(80)
	diffJoined := strings.Join(diffRendered, "\n")
	if !strings.Contains(diffJoined, "+func New() {}") || !strings.Contains(diffJoined, "-old()") {
		t.Fatalf("expected diff lines to be preserved in diff render:\n%s", diffJoined)
	}
}

func TestActivateSkillFallbackToTarget(t *testing.T) {
	cell := &ToolCell{
		Name:     "activate_skill",
		Target:   `"pdf-processing"`,
		Body:     "Loaded skill instructions successfully.",
		ToolKind: tool.KindRead,
	}
	rendered := cell.RenderWidth(80)
	joined := strings.Join(rendered, "\n")
	if !strings.Contains(joined, `"pdf-processing"`) {
		t.Fatalf("expected target skill name to appear in header:\n%s", joined)
	}
	if !strings.Contains(joined, "Activated skill") {
		t.Fatalf("expected 'Activated skill' in header:\n%s", joined)
	}
}

func TestTaskBodySuppressionInTranscript(t *testing.T) {
	if !shouldSuppressBody(tool.KindTask, "update_todo") {
		t.Fatal("expected KindTask to suppress body in transcript")
	}
	if !shouldSuppressBody(tool.KindTask, "get_todo") {
		t.Fatal("expected get_todo to suppress body in transcript")
	}
	if !shouldSuppressBody(tool.KindEdit, "write_file") {
		t.Fatal("expected KindEdit to suppress body in transcript")
	}
}

func TestEditToolUsesStructuredPatchCell(t *testing.T) {
	registry := newNamedTestRegistry(tool.Definition{
		Name:                "apply_patch",
		Description:         "apply a workspace patch",
		Kind:                tool.KindEdit,
		PermissionDetailKey: "patch",
	})
	service := newBubbleTestService(t, registry, permission.ModeAlwaysApprove, permission.Config{})
	m := newBubbleModel(
		context.Background(),
		service,
		registry,
		emptyTodoItems(),
		nil,
		newPermissionBridge(),
		"",
	)
	call, err := tool.NewCall(
		"edit-1",
		"apply_patch",
		[]byte(`{"patch":"*** Begin Patch\n*** Update File: internal/a.go\n*** End Patch"}`),
	)
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
