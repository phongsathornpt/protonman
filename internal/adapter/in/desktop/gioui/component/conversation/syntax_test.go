//go:build desktop || desktop_gio

package conversation_test

import (
	"image/color"
	"testing"

	"gioui.org/font"
	"gioui.org/unit"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/conversation"
)

func TestSplitMarkdownCodeBlocks(t *testing.T) {
	t.Run("plain text without code blocks", func(t *testing.T) {
		input := "This is a normal paragraph.\nAnd another line."
		blocks := conversation.SplitMarkdownCodeBlocks(input)
		if len(blocks) != 1 {
			t.Fatalf("len(blocks) = %d, want 1", len(blocks))
		}
		if blocks[0].Kind != conversation.MarkdownBlockText || blocks[0].Text != input {
			t.Fatalf("block[0] = %+v, want MarkdownBlockText with full input", blocks[0])
		}
	})

	t.Run("single code block", func(t *testing.T) {
		input := "Before text\n```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\nAfter text"
		blocks := conversation.SplitMarkdownCodeBlocks(input)
		if len(blocks) != 3 {
			t.Fatalf("len(blocks) = %d, want 3", len(blocks))
		}
		if blocks[0].Kind != conversation.MarkdownBlockText || blocks[0].Text != "Before text" {
			t.Errorf("blocks[0] = %+v", blocks[0])
		}
		if blocks[1].Kind != conversation.MarkdownBlockCode || blocks[1].Lang != "go" {
			t.Errorf("blocks[1] = %+v", blocks[1])
		}
		wantCode := "func main() {\n\tprintln(\"hi\")\n}"
		if blocks[1].Code != wantCode {
			t.Errorf("blocks[1].Code = %q, want %q", blocks[1].Code, wantCode)
		}
		if blocks[2].Kind != conversation.MarkdownBlockText || blocks[2].Text != "After text" {
			t.Errorf("blocks[2] = %+v", blocks[2])
		}
	})

	t.Run("multiple code blocks with different languages", func(t *testing.T) {
		input := "Rust:\n```rust\nfn main() {}\n```\nPython:\n```py\ndef main(): pass\n```"
		blocks := conversation.SplitMarkdownCodeBlocks(input)
		if len(blocks) != 4 {
			t.Fatalf("len(blocks) = %d, want 4", len(blocks))
		}
		if blocks[1].Kind != conversation.MarkdownBlockCode || blocks[1].Lang != "rust" {
			t.Errorf("block 1 = %+v", blocks[1])
		}
		if blocks[3].Kind != conversation.MarkdownBlockCode || blocks[3].Lang != "py" {
			t.Errorf("block 3 = %+v", blocks[3])
		}
	})

	t.Run("unclosed code block handles gracefully", func(t *testing.T) {
		input := "Streaming:\n```go\nfunc partial() {"
		blocks := conversation.SplitMarkdownCodeBlocks(input)
		if len(blocks) != 2 {
			t.Fatalf("len(blocks) = %d, want 2", len(blocks))
		}
		if blocks[1].Kind != conversation.MarkdownBlockCode || blocks[1].Lang != "go" || blocks[1].Code != "func partial() {" {
			t.Errorf("block 1 = %+v", blocks[1])
		}
	})
}

func TestTokenizeCode(t *testing.T) {
	t.Run("Go syntax tokens", func(t *testing.T) {
		code := "func test() string {\n\tx := 42 // comment\n\treturn \"done\"\n}"
		lines := conversation.TokenizeCode(code, "go")
		if len(lines) != 4 {
			t.Fatalf("len(lines) = %d, want 4", len(lines))
		}

		foundFunc := false
		foundString := false
		for _, tok := range lines[0] {
			if tok.Text == "func" && tok.Type == conversation.TokenKeyword {
				foundFunc = true
			}
			if tok.Text == "string" && tok.Type == conversation.TokenTypeIdent {
				foundString = true
			}
		}
		if !foundFunc {
			t.Errorf("line 0 missing func keyword token: %#v", lines[0])
		}
		if !foundString {
			t.Errorf("line 0 missing string type token: %#v", lines[0])
		}

		foundOp := false
		foundNum := false
		foundComment := false
		for _, tok := range lines[1] {
			if tok.Text == ":=" && tok.Type == conversation.TokenOperator {
				foundOp = true
			}
			if tok.Text == "42" && tok.Type == conversation.TokenNumber {
				foundNum = true
			}
			if tok.Text == "// comment" && tok.Type == conversation.TokenComment {
				foundComment = true
			}
		}
		if !foundOp || !foundNum || !foundComment {
			t.Errorf("line 1 tokens = %#v", lines[1])
		}

		foundReturn := false
		foundStr := false
		for _, tok := range lines[2] {
			if tok.Text == "return" && tok.Type == conversation.TokenKeyword {
				foundReturn = true
			}
			if tok.Text == "\"done\"" && tok.Type == conversation.TokenString {
				foundStr = true
			}
		}
		if !foundReturn || !foundStr {
			t.Errorf("line 2 tokens = %#v", lines[2])
		}
	})

	t.Run("Python syntax tokens", func(t *testing.T) {
		code := "def hello(name: str):\n    # say hi\n    return f\"hi {name}\""
		lines := conversation.TokenizeCode(code, "py")
		if len(lines) != 3 {
			t.Fatalf("len(lines) = %d, want 3", len(lines))
		}
		foundDef := false
		for _, tok := range lines[0] {
			if tok.Text == "def" && tok.Type == conversation.TokenKeyword {
				foundDef = true
			}
		}
		if !foundDef {
			t.Errorf("line 0 missing def keyword: %#v", lines[0])
		}
		foundHashComment := false
		for _, tok := range lines[1] {
			if tok.Text == "# say hi" && tok.Type == conversation.TokenComment {
				foundHashComment = true
			}
		}
		if !foundHashComment {
			t.Errorf("line 1 missing # comment: %#v", lines[1])
		}
	})

	t.Run("Rust syntax tokens", func(t *testing.T) {
		code := "pub fn run() -> Result<(), String> {\n    let mut x: i32 = 100;\n}"
		lines := conversation.TokenizeCode(code, "rust")
		if len(lines) != 3 {
			t.Fatalf("len(lines) = %d, want 3", len(lines))
		}
		foundPub := false
		foundFn := false
		foundResult := false
		for _, tok := range lines[0] {
			if tok.Text == "pub" && tok.Type == conversation.TokenKeyword {
				foundPub = true
			}
			if tok.Text == "fn" && tok.Type == conversation.TokenKeyword {
				foundFn = true
			}
			if tok.Text == "Result" && tok.Type == conversation.TokenTypeIdent {
				foundResult = true
			}
		}
		if !foundPub || !foundFn || !foundResult {
			t.Errorf("line 0 tokens = %#v", lines[0])
		}
	})
}

func TestHighlightCodeSpans(t *testing.T) {
	st := conversation.SyntaxTheme{
		Font:      font.Font{},
		FontSize:  unit.Sp(12),
		Plain:     color.NRGBA{R: 200, G: 200, B: 200, A: 255},
		Keyword:   color.NRGBA{R: 255, G: 100, B: 100, A: 255},
		TypeIdent: color.NRGBA{R: 100, G: 200, B: 100, A: 255},
		String:    color.NRGBA{R: 100, G: 100, B: 255, A: 255},
		Comment:   color.NRGBA{R: 150, G: 150, B: 150, A: 255},
		Number:    color.NRGBA{R: 255, G: 200, B: 100, A: 255},
		Operator:  color.NRGBA{R: 200, G: 150, B: 200, A: 255},
	}
	spans := conversation.HighlightCodeSpans(st, "go", "package main\n\nfunc main() {}\n")
	if len(spans) == 0 {
		t.Fatal("HighlightCodeSpans returned empty spans")
	}
	foundPkg := false
	for _, s := range spans {
		if s.Content == "package" {
			foundPkg = true
			if s.Color != st.Keyword {
				t.Errorf("package keyword color = %v, want %v", s.Color, st.Keyword)
			}
		}
	}
	if !foundPkg {
		t.Errorf("HighlightCodeSpans did not find 'package' span: %#v", spans)
	}
}

func TestHasRunePrefix(t *testing.T) {
	runes := []rune("hello world")
	for _, tc := range []struct {
		name   string
		index  int
		prefix string
		want   bool
	}{
		{name: "match at start", index: 0, prefix: "hello", want: true},
		{name: "match mid string", index: 6, prefix: "world", want: true},
		{name: "mismatch", index: 0, prefix: "world", want: false},
		{name: "single char mismatch", index: 0, prefix: "y", want: false},
		{name: "prefix extends past end", index: 8, prefix: "world", want: false},
		{name: "at end of runes", index: len(runes), prefix: "#", want: false},
		{name: "negative index", index: -1, prefix: "h", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := conversation.HasRunePrefix(runes, tc.index, []rune(tc.prefix)); got != tc.want {
				t.Fatalf("HasRunePrefix(%q, %d, %q) = %v, want %v", string(runes), tc.index, tc.prefix, got, tc.want)
			}
		})
	}
}

func TestTokenizeLineCommentDetection(t *testing.T) {
	for _, tc := range []struct {
		name        string
		line        string
		lang        string
		wantComment string
	}{
		{name: "go line comment", line: "x := 1 // trailing", lang: "go", wantComment: "// trailing"},
		{name: "python hash comment", line: "def f():  # note", lang: "py", wantComment: "# note"},
		{name: "sql dash comment", line: "SELECT 1 -- note", lang: "sql", wantComment: "-- note"},
		{name: "comment only line", line: "//", lang: "go", wantComment: "//"},
		{name: "no comment", line: "x := 1", lang: "go", wantComment: ""},
		{name: "single slash is not a comment", line: "a / b", lang: "go", wantComment: ""},
		{name: "trailing slash is not a comment", line: "path/", lang: "go", wantComment: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comment := ""
			for _, tok := range conversation.TokenizeLine(tc.line, tc.lang) {
				if tok.Type == conversation.TokenComment {
					comment = tok.Text
				}
			}
			if comment != tc.wantComment {
				t.Fatalf("comment token = %q, want %q (line %q)", comment, tc.wantComment, tc.line)
			}
		})
	}
}
