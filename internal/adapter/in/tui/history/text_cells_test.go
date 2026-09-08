package history

import (
	"reflect"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/textview"
)

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
		want := textview.RenderMarkdownLines(trimmed, 48)
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
		wantMarkdown := textview.RenderMarkdownLines(trimmed, 46)
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
