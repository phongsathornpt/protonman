package tui

import (
	"fmt"
	"testing"
)

func BenchmarkHistoryStateRenderLines_50Cells(b *testing.B) {
	state := NewHistoryState(1000)
	for i := 0; i < 25; i++ {
		state.Append(&UserCell{Text: fmt.Sprintf("User query number %d asking for assistance", i)})
		state.Append(&AssistantCell{Text: fmt.Sprintf("Assistant response number %d providing a detailed multi-line\nexplanation of the code.\nDone.", i)})
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lines := state.RenderLines()
		if len(lines) == 0 {
			b.Fatal("expected rendered lines")
		}
	}
}

func BenchmarkSanitizeBubbleText(b *testing.B) {
	input := "\x1b[31mRed text\x1b[0m with \t tabs \r\n and \x07 bells"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = sanitizeBubbleText(input)
	}
}
