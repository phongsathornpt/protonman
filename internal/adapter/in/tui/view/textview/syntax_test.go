package textview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

func TestHighlightCodeLineGo(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "go"}
	line := `func main() { fmt.Println("hello", 123) } // comment`
	highlighted := HighlightCodeLine(line, "go", state)
	plain := ansi.Strip(highlighted)

	if plain != line {
		t.Fatalf("plain text mismatch: got %q, want %q", plain, line)
	}

	// Verify keywords, strings, comments are highlighted with ANSI escape codes
	if !strings.Contains(highlighted, "\x1b[") {
		t.Fatalf("expected ANSI highlighting in output: %q", highlighted)
	}
}

func TestHighlightCodeLineGoMultilineString(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "go"}

	// Line 1 opens backtick
	l1 := "const query = `SELECT"
	h1 := HighlightCodeLine(l1, "go", state)
	if state.inMultilineStr != '`' {
		t.Fatalf("expected inMultilineStr = '`', got %c", state.inMultilineStr)
	}
	if !strings.Contains(h1, "\x1b[") {
		t.Fatalf("expected ANSI styling in line 1: %q", h1)
	}

	// Line 2 continues
	l2 := "  FROM users"
	h2 := HighlightCodeLine(l2, "go", state)
	if state.inMultilineStr != '`' {
		t.Fatalf("expected still inMultilineStr = '`', got %c", state.inMultilineStr)
	}
	if !strings.Contains(h2, "\x1b[") {
		t.Fatalf("expected ANSI styling in line 2: %q", h2)
	}

	// Line 3 closes
	l3 := "  WHERE id = 1`"
	h3 := HighlightCodeLine(l3, "go", state)
	if state.inMultilineStr != 0 {
		t.Fatalf("expected inMultilineStr = 0 after close, got %c", state.inMultilineStr)
	}
	if !strings.Contains(h3, "\x1b[") {
		t.Fatalf("expected ANSI styling in line 3: %q", h3)
	}
}

func TestHighlightCodeLineBlockComment(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "go"}

	l1 := "/* start of block comment"
	_ = HighlightCodeLine(l1, "go", state)
	if !state.inBlockComment {
		t.Fatal("expected inBlockComment = true")
	}

	l2 := "middle of comment"
	_ = HighlightCodeLine(l2, "go", state)
	if !state.inBlockComment {
		t.Fatal("expected still inBlockComment = true")
	}

	l3 := "end of comment */ var x = 10"
	_ = HighlightCodeLine(l3, "go", state)
	if state.inBlockComment {
		t.Fatal("expected inBlockComment = false after closing */")
	}
}

func TestHighlightJSON(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "json"}
	line := `{"key": "value", "count": 42, "active": true, "extra": null}`
	highlighted := HighlightCodeLine(line, "json", state)
	plain := ansi.Strip(highlighted)

	if plain != line {
		t.Fatalf("plain text mismatch: got %q, want %q", plain, line)
	}

	// Verify key and value styles differ
	keyRendered := syntaxTypeInline.prefix
	if keyRendered != "" && !strings.Contains(highlighted, keyRendered) {
		t.Fatalf("expected key to use type style in JSON: %q", highlighted)
	}
}

func TestHighlightBash(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "bash"}
	line := `if [ "$VAR" = "test" ]; then echo 1; fi # check`
	highlighted := HighlightCodeLine(line, "bash", state)
	plain := ansi.Strip(highlighted)

	if plain != line {
		t.Fatalf("plain text mismatch: got %q, want %q", plain, line)
	}
}

func TestHighlightPython(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "python"}
	line := `def add(a: int) -> int: return a + 1 # inline`
	highlighted := HighlightCodeLine(line, "python", state)
	plain := ansi.Strip(highlighted)

	if plain != line {
		t.Fatalf("plain text mismatch: got %q, want %q", plain, line)
	}
}

func TestHighlightPythonTripleQuotes(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "python"}
	l1 := `"""docstring starts`
	_ = HighlightCodeLine(l1, "python", state)
	if state.inMultilineStr != '"' {
		t.Fatalf("expected inMultilineStr = '\"', got %c", state.inMultilineStr)
	}

	l2 := `docstring ends"""`
	_ = HighlightCodeLine(l2, "python", state)
	if state.inMultilineStr != 0 {
		t.Fatalf("expected inMultilineStr = 0, got %c", state.inMultilineStr)
	}
}

func TestHighlightDiff(t *testing.T) {
	add := "+added line"
	hAdd := HighlightCodeLine(add, "diff", nil)
	if !strings.Contains(hAdd, tuistyle.DiffAddStyle.Render("+added line")) {
		t.Fatalf("diff add mismatch: got %q", hAdd)
	}

	del := "-deleted line"
	hDel := HighlightCodeLine(del, "diff", nil)
	if !strings.Contains(hDel, tuistyle.DiffDeleteStyle.Render("-deleted line")) {
		t.Fatalf("diff delete mismatch: got %q", hDel)
	}

	hunk := "@@ -1,3 +1,3 @@"
	hHunk := HighlightCodeLine(hunk, "diff", nil)
	if !strings.Contains(hHunk, tuistyle.DiffHunkStyle.Render(hunk)) {
		t.Fatalf("diff hunk mismatch: got %q", hHunk)
	}
}

func TestHighlightSQL(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "sql"}
	line := `SELECT id, name FROM users WHERE active = TRUE AND count > 5; -- comment`
	highlighted := HighlightCodeLine(line, "sql", state)
	plain := ansi.Strip(highlighted)

	if plain != line {
		t.Fatalf("plain text mismatch: got %q, want %q", plain, line)
	}
}

func TestHighlightHTML(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "html"}
	line := `<div class="container" id="main"><!-- comment -->Hello</div>`
	highlighted := HighlightCodeLine(line, "html", state)
	plain := ansi.Strip(highlighted)

	if plain != line {
		t.Fatalf("plain text mismatch: got %q, want %q", plain, line)
	}
}

func TestHighlightCSS(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "css"}
	line := `.btn { display: flex; color: #fff; margin: 10px; } /* comment */`
	highlighted := HighlightCodeLine(line, "css", state)
	plain := ansi.Strip(highlighted)

	if plain != line {
		t.Fatalf("plain text mismatch: got %q, want %q", plain, line)
	}
}

func TestHighlightYAML(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: "yaml"}
	line := `name: "protonman"`
	highlighted := HighlightCodeLine(line, "yaml", state)
	plain := ansi.Strip(highlighted)

	if plain != line {
		t.Fatalf("plain text mismatch: got %q, want %q", plain, line)
	}
}

func TestHighlightGeneric(t *testing.T) {
	state := &MarkdownState{inFence: true, fenceLang: ""}
	line := `x = 100 // generic comment`
	highlighted := HighlightCodeLine(line, "", state)
	plain := ansi.Strip(highlighted)

	if plain != line {
		t.Fatalf("plain text mismatch: got %q, want %q", plain, line)
	}
}

func TestRenderMarkdownLinesFencedCodeFrame(t *testing.T) {
	md := "```go\npackage main\n\nfunc main() {}\n```"
	lines := RenderMarkdownLines(md, 80)
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = ansi.Strip(l)
	}

	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d: %#v", len(lines), plain)
	}

	// Header: ╭─ go
	if !strings.Contains(plain[0], "╭─ go") {
		t.Fatalf("expected header with '╭─ go', got %q", plain[0])
	}

	// Gutter: │
	if !strings.HasPrefix(strings.TrimSpace(plain[1]), "│") {
		t.Fatalf("expected gutter '│' on code lines, got %q", plain[1])
	}

	// Footer: ╰─
	lastLine := plain[len(plain)-1]
	if !strings.Contains(lastLine, "╰─") {
		t.Fatalf("expected footer with '╰─', got %q", lastLine)
	}
}

func TestRenderMarkdownLinesUnterminated(t *testing.T) {
	md := "```python\nprint('hello')\n"
	lines := RenderMarkdownLines(md, 80)
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = ansi.Strip(l)
	}

	lastLine := plain[len(plain)-1]
	if !strings.Contains(lastLine, "╰─ python (unterminated)") {
		t.Fatalf("expected unterminated footer '╰─ python (unterminated)', got %q", lastLine)
	}
}
