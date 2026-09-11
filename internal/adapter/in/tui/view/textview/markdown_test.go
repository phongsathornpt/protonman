package textview

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/style"
)

func TestMarkdownHeadingLevelsAreDistinct(t *testing.T) {
	want := map[string]int{"# One": 1, "## Two": 2, "### Three": 3, "#### Four": 4}
	for line, wantLevel := range want {
		text, level, ok := markdownHeading(line)
		if !ok {
			t.Fatalf("markdownHeading(%q) not recognized", line)
		}
		if text == "" || level != wantLevel {
			t.Fatalf("markdownHeading(%q) = %q,%d, want level %d", line, text, level, wantLevel)
		}
	}

	h1 := markdownHeadingStyleForLevel(1).GetForeground()
	h2 := markdownHeadingStyleForLevel(2).GetForeground()
	h3 := markdownHeadingStyleForLevel(3).GetForeground()
	if reflect.DeepEqual(h1, h2) || reflect.DeepEqual(h2, h3) || reflect.DeepEqual(h1, h3) {
		t.Fatalf("heading levels share a color: h1=%v h2=%v h3=%v", h1, h2, h3)
	}
	if !reflect.DeepEqual(h1, tuistyle.ColorTextPrimary) {
		t.Fatalf("h1 foreground = %v, want %v", h1, tuistyle.ColorTextPrimary)
	}
	if level := markdownHeadingStyleForLevel(6); !reflect.DeepEqual(level.GetForeground(), tuistyle.ColorTextTertiary) {
		t.Fatalf("h3+ foreground = %v, want %v", level.GetForeground(), tuistyle.ColorTextTertiary)
	}
}

func TestProseMeasureCapsBodyWidth(t *testing.T) {
	if got := proseWidth(200); got != tuistyle.MeasureProse {
		t.Fatalf("proseWidth(200) = %d, want %d", got, tuistyle.MeasureProse)
	}
	if got := proseWidth(60); got != 60 {
		t.Fatalf("proseWidth(60) = %d, want 60", got)
	}

	text := strings.TrimSpace(strings.Repeat("word ", 60))
	for _, line := range RenderMarkdownBodyWrapped(text, 200) {
		if got := Width(line); got > tuistyle.MeasureProse {
			t.Fatalf("prose line width = %d, want <= %d: %q", got, tuistyle.MeasureProse, line)
		}
	}
}

func TestFencedCodeKeepsFullWidth(t *testing.T) {
	code := strings.Repeat("x", 160)
	lines := RenderMarkdownLines("```\n"+code+"\n```", 200)
	exceeds := false
	for _, line := range lines {
		if Width(line) > tuistyle.MeasureProse {
			exceeds = true
		}
	}
	if !exceeds {
		t.Fatalf("fenced code was capped to the prose measure: %#v", lines)
	}
}

func TestInlineEmphasisDropsAsterisks(t *testing.T) {
	cases := []struct {
		in          string
		wantContain string
		wantAster   bool
	}{
		{"*italic*", "italic", false},
		{"a *b* c", "a b c", false},
		{"**bold**", "bold", false},
		{"keep `a*b` literal", "a*b", true},
		{"2 * 3 = 6", "2 * 3 = 6", true},
		{"*unclosed", "*unclosed", true},
		{"* a *", "* a *", true},
		{"***", "***", true},
	}
	for _, tc := range cases {
		plain := ansi.Strip(styleInlineMarkdown(tc.in))
		if !strings.Contains(plain, tc.wantContain) {
			t.Fatalf("styleInlineMarkdown(%q) = %q, want containing %q", tc.in, plain, tc.wantContain)
		}
		if gotAster := strings.Contains(plain, "*"); gotAster != tc.wantAster {
			t.Fatalf("styleInlineMarkdown(%q) = %q asterisk=%v, want %v", tc.in, plain, gotAster, tc.wantAster)
		}
	}
}
