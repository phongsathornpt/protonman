//go:build desktop || desktop_gio

package gioui

import (
	"testing"
)

func TestSplitMarkdownCodeBlocks(t *testing.T) {
	t.Run("plain text without code blocks", func(t *testing.T) {
		input := "This is a normal paragraph.\nAnd another line."
		blocks := splitMarkdownCodeBlocks(input)
		if len(blocks) != 1 {
			t.Fatalf("len(blocks) = %d, want 1", len(blocks))
		}
		if blocks[0].kind != markdownBlockText || blocks[0].text != input {
			t.Fatalf("block[0] = %+v, want markdownBlockText with full input", blocks[0])
		}
	})

	t.Run("single code block", func(t *testing.T) {
		input := "Before text\n```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\nAfter text"
		blocks := splitMarkdownCodeBlocks(input)
		if len(blocks) != 3 {
			t.Fatalf("len(blocks) = %d, want 3", len(blocks))
		}
		if blocks[0].kind != markdownBlockText || blocks[0].text != "Before text" {
			t.Errorf("blocks[0] = %+v", blocks[0])
		}
		if blocks[1].kind != markdownBlockCode || blocks[1].lang != "go" {
			t.Errorf("blocks[1] = %+v", blocks[1])
		}
		wantCode := "func main() {\n\tprintln(\"hi\")\n}"
		if blocks[1].code != wantCode {
			t.Errorf("blocks[1].code = %q, want %q", blocks[1].code, wantCode)
		}
		if blocks[2].kind != markdownBlockText || blocks[2].text != "After text" {
			t.Errorf("blocks[2] = %+v", blocks[2])
		}
	})

	t.Run("multiple code blocks with different languages", func(t *testing.T) {
		input := "Rust:\n```rust\nfn main() {}\n```\nPython:\n```py\ndef main(): pass\n```"
		blocks := splitMarkdownCodeBlocks(input)
		if len(blocks) != 4 {
			t.Fatalf("len(blocks) = %d, want 4", len(blocks))
		}
		if blocks[1].kind != markdownBlockCode || blocks[1].lang != "rust" {
			t.Errorf("block 1 = %+v", blocks[1])
		}
		if blocks[3].kind != markdownBlockCode || blocks[3].lang != "py" {
			t.Errorf("block 3 = %+v", blocks[3])
		}
	})

	t.Run("unclosed code block handles gracefully", func(t *testing.T) {
		input := "Streaming:\n```go\nfunc partial() {"
		blocks := splitMarkdownCodeBlocks(input)
		if len(blocks) != 2 {
			t.Fatalf("len(blocks) = %d, want 2", len(blocks))
		}
		if blocks[1].kind != markdownBlockCode || blocks[1].lang != "go" || blocks[1].code != "func partial() {" {
			t.Errorf("block 1 = %+v", blocks[1])
		}
	})
}

func TestTokenizeCode(t *testing.T) {
	t.Run("Go syntax tokens", func(t *testing.T) {
		code := "func test() string {\n\tx := 42 // comment\n\treturn \"done\"\n}"
		lines := tokenizeCode(code, "go")
		if len(lines) != 4 {
			t.Fatalf("len(lines) = %d, want 4", len(lines))
		}

		// Line 0: func test() string {
		foundFunc := false
		foundString := false
		for _, tok := range lines[0] {
			if tok.Text == "func" && tok.Type == TokenKeyword {
				foundFunc = true
			}
			if tok.Text == "string" && tok.Type == TokenTypeIdent {
				foundString = true
			}
		}
		if !foundFunc {
			t.Errorf("line 0 missing func keyword token: %#v", lines[0])
		}
		if !foundString {
			t.Errorf("line 0 missing string type token: %#v", lines[0])
		}

		// Line 1: x := 42 // comment
		foundOp := false
		foundNum := false
		foundComment := false
		for _, tok := range lines[1] {
			if tok.Text == ":=" && tok.Type == TokenOperator {
				foundOp = true
			}
			if tok.Text == "42" && tok.Type == TokenNumber {
				foundNum = true
			}
			if tok.Text == "// comment" && tok.Type == TokenComment {
				foundComment = true
			}
		}
		if !foundOp || !foundNum || !foundComment {
			t.Errorf("line 1 tokens = %#v", lines[1])
		}

		// Line 2: return "done"
		foundReturn := false
		foundStr := false
		for _, tok := range lines[2] {
			if tok.Text == "return" && tok.Type == TokenKeyword {
				foundReturn = true
			}
			if tok.Text == "\"done\"" && tok.Type == TokenString {
				foundStr = true
			}
		}
		if !foundReturn || !foundStr {
			t.Errorf("line 2 tokens = %#v", lines[2])
		}
	})

	t.Run("Python syntax tokens", func(t *testing.T) {
		code := "def hello(name: str):\n    # say hi\n    return f\"hi {name}\""
		lines := tokenizeCode(code, "py")
		if len(lines) != 3 {
			t.Fatalf("len(lines) = %d, want 3", len(lines))
		}
		foundDef := false
		for _, tok := range lines[0] {
			if tok.Text == "def" && tok.Type == TokenKeyword {
				foundDef = true
			}
		}
		if !foundDef {
			t.Errorf("line 0 missing def keyword: %#v", lines[0])
		}
		foundHashComment := false
		for _, tok := range lines[1] {
			if tok.Text == "# say hi" && tok.Type == TokenComment {
				foundHashComment = true
			}
		}
		if !foundHashComment {
			t.Errorf("line 1 missing # comment: %#v", lines[1])
		}
	})

	t.Run("Rust syntax tokens", func(t *testing.T) {
		code := "pub fn run() -> Result<(), String> {\n    let mut x: i32 = 100;\n}"
		lines := tokenizeCode(code, "rust")
		if len(lines) != 3 {
			t.Fatalf("len(lines) = %d, want 3", len(lines))
		}
		foundPub := false
		foundFn := false
		foundResult := false
		for _, tok := range lines[0] {
			if tok.Text == "pub" && tok.Type == TokenKeyword {
				foundPub = true
			}
			if tok.Text == "fn" && tok.Type == TokenKeyword {
				foundFn = true
			}
			if tok.Text == "Result" && tok.Type == TokenTypeIdent {
				foundResult = true
			}
		}
		if !foundPub || !foundFn || !foundResult {
			t.Errorf("line 0 tokens = %#v", lines[0])
		}
	})
}

func TestHighlightCodeSpans(t *testing.T) {
	th := newTheme("dark")
	spans := highlightCodeSpans(th, "go", "package main\n\nfunc main() {}\n")
	if len(spans) == 0 {
		t.Fatal("highlightCodeSpans returned empty spans")
	}
	foundPkg := false
	for _, s := range spans {
		if s.Content == "package" {
			foundPkg = true
			if s.Color != th.primary {
				t.Errorf("package keyword color = %v, want %v", s.Color, th.primary)
			}
		}
	}
	if !foundPkg {
		t.Errorf("highlightCodeSpans did not find 'package' span: %#v", spans)
	}
}
