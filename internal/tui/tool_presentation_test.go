package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/projectTHORN/proton/internal/tool"
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
			kind:       tool.KindWebFetch,
			args:       `{"url":"https://protonman.dev"}`,
			wantTarget: "https://protonman.dev",
			wantKind:   tool.KindWebFetch,
		},
		{
			name:       "web_search query",
			toolName:   "web_search",
			kind:       tool.KindWebSearch,
			args:       `{"query":"proton AI"}`,
			wantTarget: `"proton AI"`,
			wantKind:   tool.KindWebSearch,
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
			args:       `{"path":"cmd/proton"}`,
			wantTarget: "cmd/proton",
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
			args:       `{"profile":"explorer","task":"find all authentication handlers"}`,
			wantTarget: "[explorer] find all authentication handlers",
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
			gotTarget, gotKind := extractToolTarget(tt.toolName, tt.kind, json.RawMessage(tt.args))
			if gotTarget != tt.wantTarget {
				t.Errorf("extractToolTarget() target = %q, want %q", gotTarget, tt.wantTarget)
			}
			if tt.wantKind != "" && gotKind != tt.wantKind {
				t.Errorf("extractToolTarget() kind = %q, want %q", gotKind, tt.wantKind)
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
	summary := summarizeListDir(dirContent)
	if !strings.Contains(summary, "5 items") || !strings.Contains(summary, "2 dirs") || !strings.Contains(summary, "3 files") {
		t.Fatalf("unexpected list_dir summary: %s", summary)
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

	dirtyStatus := " M internal/tui/theme.go\n?? new_file.go\nM  cmd/proton/main.go\n"
	got := summarizeGitStatus(dirtyStatus)
	if !strings.Contains(got, "1 staged") || !strings.Contains(got, "1 modified") || !strings.Contains(got, "1 untracked") {
		t.Fatalf("unexpected git status summary: %s", got)
	}
}

func TestFormatOutputFold(t *testing.T) {
	lines := []string{"one", "two", "three"}
	if got := formatOutputFold(lines, 3); len(got) != 3 {
		t.Fatalf("expected unfoled lines for len <= 3, got: %d", len(got))
	}

	longLines := []string{"line1", "line2", "line3", "line4", "line5", "line6", "line7", "line8"}
	folded := formatOutputFold(longLines, 3)
	if len(folded) != 4 { // first 2 + fold indicator + last 1
		t.Fatalf("expected 4 folded items, got: %d (%v)", len(folded), folded)
	}
	if !strings.Contains(folded[2], "5 lines hidden") || !strings.Contains(folded[2], "ctrl+t") {
		t.Fatalf("expected fold indicator with hidden count, got: %s", folded[2])
	}
}
