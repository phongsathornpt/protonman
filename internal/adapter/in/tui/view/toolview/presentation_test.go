package toolview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

func TestExtractToolTarget(t *testing.T) {
	tests := []struct {
		name       string
		toolName   string
		kind       tool.Kind
		args       string
		wantTarget string
		wantKind   tool.Kind
	}{
		{
			name:       "web_fetch url",
			toolName:   "web_fetch",
			kind:       tool.KindWeb,
			args:       `{"url":"https://protonman.dev"}`,
			wantTarget: "https://protonman.dev",
			wantKind:   tool.KindWeb,
		},
		{
			name:       "web_search query",
			toolName:   "web_search",
			kind:       tool.KindWeb,
			args:       `{"query":"proton AI"}`,
			wantTarget: `"proton AI"`,
			wantKind:   tool.KindWeb,
		},
		{
			name:       "read_file path",
			toolName:   "read_file",
			kind:       tool.KindRead,
			args:       `{"path":"internal/tui/theme.go"}`,
			wantTarget: "internal/tui/theme.go",
			wantKind:   tool.KindRead,
		},
		{
			name:       "list_dir with path",
			toolName:   "list_dir",
			kind:       tool.KindRead,
			args:       `{"path":"cmd/protonman"}`,
			wantTarget: "cmd/protonman",
			wantKind:   tool.KindRead,
		},
		{
			name:       "list_dir default path",
			toolName:   "list_dir",
			kind:       tool.KindRead,
			args:       `{}`,
			wantTarget: ".",
			wantKind:   tool.KindRead,
		},
		{
			name:       "grep pattern and path",
			toolName:   "grep",
			kind:       tool.KindGrep,
			args:       `{"pattern":"glyphMark","path":"internal/tui"}`,
			wantTarget: `"glyphMark" in internal/tui`,
			wantKind:   tool.KindGrep,
		},
		{
			name:       "grep pattern only",
			toolName:   "grep",
			kind:       tool.KindGrep,
			args:       `{"pattern":"glyphMark"}`,
			wantTarget: `"glyphMark"`,
			wantKind:   tool.KindGrep,
		},
		{
			name:       "bash command",
			toolName:   "bash",
			kind:       tool.KindBash,
			args:       `{"command":"git status"}`,
			wantTarget: "git status",
			wantKind:   tool.KindBash,
		},
		{
			name:       "write_file path",
			toolName:   "write_file",
			kind:       tool.KindEdit,
			args:       `{"path":"main.go","content":"package main"}`,
			wantTarget: "main.go",
			wantKind:   tool.KindEdit,
		},
		{
			name:       "activate_skill name",
			toolName:   "activate_skill",
			kind:       "",
			args:       `{"name":"golang-pro"}`,
			wantTarget: `"golang-pro"`,
			wantKind:   "",
		},
		{
			name:       "delegate_task profile and task",
			toolName:   "delegate_task",
			kind:       "",
			args:       `{"profile":"int","task":"find all authentication handlers"}`,
			wantTarget: "[int] find all authentication handlers",
			wantKind:   "",
		},
		{
			name:       "checkpoint_restore id",
			toolName:   "checkpoint_restore",
			kind:       "",
			args:       `{"checkpoint_id":"chk-12345"}`,
			wantTarget: "chk-12345",
			wantKind:   "",
		},
		{
			name:       "mcp heuristic scan",
			toolName:   "github_search_issues",
			kind:       tool.KindMCP,
			args:       `{"query":"is:issue is:open"}`,
			wantTarget: "is:issue is:open",
			wantKind:   tool.KindMCP,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTarget, gotKind := ExtractTarget(tt.toolName, tt.kind, json.RawMessage(tt.args))
			if gotTarget != tt.wantTarget {
				t.Errorf("ExtractTarget() target = %q, want %q", gotTarget, tt.wantTarget)
			}
			if tt.wantKind != "" && gotKind != tt.wantKind {
				t.Errorf("ExtractTarget() kind = %q, want %q", gotKind, tt.wantKind)
			}
		})
	}
}

func TestSummarizeWebFetchHTML(t *testing.T) {
	// Real HTML payload similar to the user's screenshot
	htmlPayload := `<!DOCTYPE html>
<html lang="th">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover" />
    <title>รวม AI ราคาถูกใน API เดียว | protonmanAI</title>
    <meta name="description" content="รวม AI ราคาถูกอย่าง DeepSeek, Qwen และโมเดล AI ชั้นนำไว้ใน API Key เดียว แบบเหมาจ่าย ช่วยลดต้นทุน AI สำหรับนักพัฒนาและทีม" />
  </head>
  <body><h1>Welcome</h1></body>
</html>`

	summary := summarizeWebFetch(htmlPayload, false)
	if !strings.Contains(summary, "รวม AI ราคาถูกใน API เดียว | protonmanAI") {
		t.Fatalf("expected summary to contain page title, got: %s", summary)
	}
	if !strings.Contains(summary, "B") {
		t.Fatalf("expected summary to contain byte size, got: %s", summary)
	}
}

func TestSummarizeWebFetchJSON(t *testing.T) {
	jsonPayload := `{"items":[{"id":1},{"id":2},{"id":3}],"total":3}`
	summary := summarizeWebFetch(jsonPayload, false)
	if !strings.Contains(summary, "JSON object") || !strings.Contains(summary, "2 keys") {
		t.Fatalf("expected JSON object summary with 2 keys, got: %s", summary)
	}

	jsonArray := `[{"id":1},{"id":2}]`
	summaryArr := summarizeWebFetch(jsonArray, false)
	if !strings.Contains(summaryArr, "JSON array") || !strings.Contains(summaryArr, "2 items") {
		t.Fatalf("expected JSON array summary with 2 items, got: %s", summaryArr)
	}
}

func TestSummarizeReadFile(t *testing.T) {
	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	summary := summarizeReadFile(content, false)
	if !strings.Contains(summary, "6 lines") {
		t.Fatalf("expected line count in summary, got: %s", summary)
	}
}

func TestSummarizeListDir(t *testing.T) {
	dirContent := "dir  cmd/\ndir  internal/\nfile main.go\nfile go.mod\nfile README.md\n"
	summary := summarizeListDir(dirContent, false)
	if !strings.Contains(summary, "5 items") || !strings.Contains(summary, "2 dirs") || !strings.Contains(summary, "3 files") {
		t.Fatalf("unexpected list_dir summary: %s", summary)
	}

	truncatedSummary := summarizeListDir(dirContent, true)
	if !strings.Contains(truncatedSummary, "+ truncated") {
		t.Fatalf("expected '+ truncated' in summary, got: %s", truncatedSummary)
	}
}

func TestSummarizeGrep(t *testing.T) {
	grepOutput := "internal/tui/theme.go:7:glyphMark = \"◆ \"\ninternal/tui/history_cells.go:87:prefix = glyphMark\n"
	summary := summarizeGrep(grepOutput, false)
	if !strings.Contains(summary, "2 matches across 2 files") {
		t.Fatalf("unexpected grep summary: %s", summary)
	}

	emptyGrep := ""
	if got := summarizeGrep(emptyGrep, false); got != "no matches found" {
		t.Fatalf("expected 'no matches found', got: %s", got)
	}
}

func TestSummarizeGitStatus(t *testing.T) {
	cleanStatus := "nothing to commit, working tree clean\n"
	if got := summarizeGitStatus(cleanStatus); got != "working tree clean" {
		t.Fatalf("expected 'working tree clean', got: %s", got)
	}

	dirtyStatus := " M internal/tui/theme.go\n?? new_file.go\nM  cmd/protonman/main.go\n"
	got := summarizeGitStatus(dirtyStatus)
	if !strings.Contains(got, "1 staged") || !strings.Contains(got, "1 modified") || !strings.Contains(got, "1 untracked") {
		t.Fatalf("unexpected git status summary: %s", got)
	}
}

func TestFormatOutputFold(t *testing.T) {
	lines := []string{"one", "two", "three"}
	if got := FormatOutputFold(lines, 3); len(got) != 3 {
		t.Fatalf("expected unfolded lines for len <= 3, got: %d", len(got))
	}

	fourLines := []string{"one", "two", "three", "four"}
	fourFolded := FormatOutputFold(fourLines, 3)
	if len(fourFolded) != 4 {
		t.Fatalf("expected 4 folded items, got: %d", len(fourFolded))
	}
	if !strings.Contains(fourFolded[2], "1 line hidden") {
		t.Fatalf("expected '1 line hidden', got: %s", fourFolded[2])
	}

	fiveLines := []string{"one", "two", "three", "four", "five"}
	fiveFolded := FormatOutputFold(fiveLines, 3)
	if len(fiveFolded) != 4 {
		t.Fatalf("expected 5 lines to fold to 4 items, got: %d", len(fiveFolded))
	}
	if !strings.Contains(fiveFolded[2], "2 lines hidden") {
		t.Fatalf("expected '2 lines hidden', got: %s", fiveFolded[2])
	}

	longLines := []string{"line1", "line2", "line3", "line4", "line5", "line6", "line7", "line8"}
	folded := FormatOutputFold(longLines, 3)
	if len(folded) != 4 { // first 2 + fold indicator + last 1
		t.Fatalf("expected 4 folded items, got: %d (%v)", len(folded), folded)
	}
	if !strings.Contains(folded[2], "5 lines hidden") || !strings.Contains(folded[2], "ctrl+t") {
		t.Fatalf("expected fold indicator with hidden count, got: %s", folded[2])
	}
}

func TestStyleDiffLine(t *testing.T) {
	styled, isDiff := StyleDiffLine("+func NewFeature() {")
	if !isDiff || !strings.Contains(styled, "+func NewFeature() {") {
		t.Fatalf("expected diff addition styling, got: %s (isDiff=%v)", styled, isDiff)
	}

	styledDel, isDiffDel := StyleDiffLine("-oldCode()")
	if !isDiffDel || !strings.Contains(styledDel, "-oldCode()") {
		t.Fatalf("expected diff deletion styling, got: %s (isDiff=%v)", styledDel, isDiffDel)
	}

	styledHunk, isDiffHunk := StyleDiffLine("@@ -10,5 +10,6 @@")
	if !isDiffHunk || !strings.Contains(styledHunk, "@@ -10,5 +10,6 @@") {
		t.Fatalf("expected diff hunk styling, got: %s (isDiff=%v)", styledHunk, isDiffHunk)
	}

	_, isRegular := StyleDiffLine("regular terminal output")
	if isRegular {
		t.Fatal("expected regular output not to be classified as diff")
	}
}

func TestSummarizeEdit(t *testing.T) {
	if got := summarizeEdit("write_file", "Wrote file successfully to /path/to/main.go."); got != "saved" {
		t.Fatalf("expected 'saved' for write_file, got: %s", got)
	}
	if got := summarizeEdit("search_replace", "The file foo.go has been updated."); got != "1 replacement applied" {
		t.Fatalf("expected '1 replacement applied' for search_replace, got: %s", got)
	}
	if got := summarizeEdit("apply_patch", "Success. Updated the following files:"); got != "patch applied" {
		t.Fatalf("expected 'patch applied' for apply_patch, got: %s", got)
	}
	if got := summarizeEdit("checkpoint_restore", "Restored checkpoint cp-1."); got != "restored checkpoint" {
		t.Fatalf("expected 'restored checkpoint' for checkpoint_restore, got: %s", got)
	}
}

func TestDetectFileType(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"main.go", "Go"},
		{"config.json", "JSON"},
		{"config.toml", "TOML"},
		{"ci.yaml", "YAML"},
		{"ci.yml", "YAML"},
		{"README.md", "Markdown"},
		{"index.ts", "TypeScript"},
		{"app.jsx", "JavaScript"},
		{"script.py", "Python"},
		{"lib.rs", "Rust"},
		{"run.sh", "Shell"},
		{"schema.sql", "SQL"},
		{"Dockerfile", "Dockerfile"},
		{"Makefile", "Makefile"},
		{"unknown.xyz", ""},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := detectFileType(tc.path)
			if got != tc.want {
				t.Errorf("detectFileType(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestFormatPathSegmentsStyled(t *testing.T) {
	// Nested path contains dir and file
	nested := FormatPath("internal/tui/theme.go")
	if !strings.Contains(nested, "internal/tui/") || !strings.Contains(nested, "theme.go") {
		t.Fatalf("expected nested path to contain dir and base, got: %s", nested)
	}

	// Root file
	rootFile := FormatPath("README.md")
	if !strings.Contains(rootFile, "README.md") {
		t.Fatalf("expected root file to contain name, got: %s", rootFile)
	}

	// Grep pattern query
	grepQuery := FormatPath(`"glyphMark" in internal/tui`)
	if !strings.Contains(grepQuery, `"glyphMark"`) || !strings.Contains(grepQuery, "internal/") {
		t.Fatalf("expected grep pattern query to contain pattern and path, got: %s", grepQuery)
	}
}

func TestExtractReadFileExcerpt(t *testing.T) {
	// Go package
	goCode := "// Package foo\npackage foo\n\nfunc main() {}\n"
	if got := ExtractReadFileExcerpt(goCode); got != "package foo" {
		t.Fatalf("expected package foo excerpt, got: %q", got)
	}

	// Markdown title
	mdText := "# Protonman Coding Agent\n\nHigh performance..."
	if got := ExtractReadFileExcerpt(mdText); got != "# Protonman Coding Agent" {
		t.Fatalf("expected markdown title excerpt, got: %q", got)
	}

	// Shell script
	shText := "#!/usr/bin/env bash\nset -euo pipefail\n"
	if got := ExtractReadFileExcerpt(shText); got != "#!/usr/bin/env bash" {
		t.Fatalf("expected shebang excerpt, got: %q", got)
	}

	// Plain statements should not be extracted
	plainCode := "x := 1\ny := 2\n"
	if got := ExtractReadFileExcerpt(plainCode); got != "" {
		t.Fatalf("expected empty excerpt for non-structural code, got: %q", got)
	}
}

func TestSummarizeReadFileTarget(t *testing.T) {
	content := "line 1\nline 2\nline 3\n"
	summary := summarizeReadFileTarget("internal/tui/theme.go", content, false)
	if !strings.Contains(summary, "4 lines") || !strings.Contains(summary, "Go") {
		t.Fatalf("expected line count and Go badge in summary, got: %s", summary)
	}

	emptySummary := summarizeReadFileTarget("empty.txt", "", false)
	if !strings.Contains(emptySummary, "0 B (empty file)") {
		t.Fatalf("expected empty file summary, got: %s", emptySummary)
	}
}

func TestTodoToolPresentation(t *testing.T) {
	target, kind := ExtractTarget("update_todo", "", json.RawMessage(`{"operations":[{"op":"set_status","id":"a","status":"in_progress"},{"op":"set_status","id":"b","status":"completed"}]}`))
	if kind != tool.KindTask || target != "2 task operations" {
		t.Fatalf("target=%q kind=%q", target, kind)
	}
	if glyph := KindGlyph(kind, "update_todo"); glyph != tuistyle.GlyphTodoActive {
		t.Fatalf("glyph = %q", glyph)
	}
	summary := SummarizeOutput("update_todo", kind, target, `{"total":2,"completed":1,"in_progress":1}`, nil, false)
	if got := summarizeTodoUpdate(`{"total":3,"completed":1,"in_progress":1,"changes":{"completed":1,"started":1,"removed":1}}`); got != "Tasks updated · 1 completed · 1 started · 1 removed" {
		t.Fatalf("todo diff summary=%q", got)
	}
	if summary != "Tasks updated · 1/2 complete · 1 active" {
		t.Fatalf("summary = %q", summary)
	}
}

func TestAgentToolPresentation(t *testing.T) {
	target, kind := ExtractTarget("delegate_task", "", json.RawMessage(`{"profile":"int","task":"inspect router behavior"}`))
	if kind != tool.KindAgent || !strings.Contains(target, "[int]") {
		t.Fatalf("target=%q kind=%q", target, kind)
	}
	if glyph := KindGlyph(kind, "delegate_task"); glyph != tuistyle.GlyphAgent {
		t.Fatalf("glyph=%q", glyph)
	}
	if got := SummarizeOutput("delegate_task", kind, target, `{"agent_id":"explorer-7","status":"queued"}`, nil, false); got != "spawned explorer-7 · queued" {
		t.Fatalf("spawn summary=%q", got)
	}
	if got := SummarizeOutput("wait_agent", kind, "", `{"timed_out":true,"event":null,"agents":[]}`, nil, false); got != "no new agent activity" {
		t.Fatalf("wait timeout summary=%q", got)
	}
	if got := SummarizeOutput("wait_agent", kind, "", `{"timed_out":false,"event":{"kind":"agent_completed","agent_id":"explorer-7"},"agents":[]}`, nil, false); got != "explorer-7 · agent_completed" {
		t.Fatalf("completed wait summary=%q", got)
	}
	if got := SummarizeOutput("wait_agent", kind, "", `{"timed_out":false,"event":{"kind":"agent_failed","agent_id":"strength-8"},"events":[{"kind":"agent_completed","agent_id":"explorer-7"},{"kind":"agent_failed","agent_id":"strength-8"}],"agents":[]}`, nil, false); got != "2 agent lifecycle events" {
		t.Fatalf("batched wait summary=%q", got)
	}
	if got := SummarizeOutput("list_agents", kind, "subagents", `{"agents":[{"id":"a","state":"canceling"},{"id":"b","state":"completed"}]}`, nil, false); got != "2 agents · 1 active" {
		t.Fatalf("canceling list summary=%q", got)
	}
	if got := SummarizeOutput("list_agents", kind, "subagents", `{"agents":[{"id":"a","state":"running"},{"id":"b","state":"completed"}]}`, nil, false); got != "2 agents · 1 active" {
		t.Fatalf("list summary=%q", got)
	}
}

func TestFormatGrepToolView(t *testing.T) {
	lines := []string{
		"internal/tui/brand.go:18:func brandLockup(width int) string {",
		"internal/tui/brand.go:31:func brandLockupWidth(width int) int {",
		"internal/tui/welcome.go:4:return brandLockup(m.width)",
		"internal/tui/theme.go:49:brandStyle = lipgloss.NewStyle()",
		"internal/tui/theme.go:50:brandMarkStyle = lipgloss.NewStyle()",
	}
	target := `"brandLockup|brandStyle"`
	view := FormatGrepView(lines, target, 80)
	if len(view) != 5 {
		t.Fatalf("expected 5 lines in folded view, got %d", len(view))
	}
	first := ansi.Strip(view[0])
	if !strings.Contains(first, "internal/tui/brand.go") || !strings.Contains(first, ":18:") {
		t.Fatalf("unexpected first line: %q", first)
	}
	if !strings.Contains(view[3], "line hidden") && !strings.Contains(view[3], "lines hidden") {
		t.Fatalf("expected fold hint, got %q", view[3])
	}
}

func TestLongPatternTruncation(t *testing.T) {
	longTarget := `"brandLockup|welcomeCard|glyphBrand|showWelcome|brandStyle|brandMark" in internal/tui`
	rendered := FormatPath(longTarget)
	if !strings.Contains(rendered, "…") {
		t.Fatalf("expected long target to be truncated with ellipsis, got: %s", rendered)
	}
}

func TestSummarizeAgentResume(t *testing.T) {
	got := SummarizeOutput("resume_agent", tool.KindAgent, "strength-4", `{"resumed_from":"strength-4","agent_id":"strength-9","profile":"strength","status":"queued"}`, nil, false)
	if got != "resumed strength-4 as strength-9 · queued" {
		t.Fatalf("summary = %q", got)
	}
}

func TestSummarizeReadFilePreservesArtifactSummary(t *testing.T) {
	image := "image png 1207x210 · sampled 64000 px · brightness mean 42.8 · dominant #505070"
	if got := summarizeReadFileTarget("screenshot.png", image, false); got != image {
		t.Fatalf("image summary = %q", got)
	}
	structured := "structured csv · rows 120 · columns 4"
	if got := summarizeReadFileTarget("bench.csv", structured, false); got != structured {
		t.Fatalf("structured summary = %q", got)
	}
}
