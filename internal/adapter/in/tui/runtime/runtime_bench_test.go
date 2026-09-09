package runtime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	applicationturn "github.com/phongsathornpt/protonman/internal/engine/turn"
)

func BenchmarkHistoryStateRenderLines50Cells(b *testing.B) {
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

func BenchmarkHistoryStateRenderLinesActiveMarkdown20KB(b *testing.B) {
	state := NewHistoryState(50000)
	for i := 0; i < 100; i++ {
		state.Append(&AssistantCell{Text: fmt.Sprintf("Committed response %d with **bold** and `code`.", i)})
	}
	state.AppendAssistantDelta(strings.Repeat("A paragraph with **bold text**, `inline code`, and [a link](https://example.com).\n", 250))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = state.RenderLines()
	}
}

func BenchmarkHistoryStateRenderLinesAt100Cells(b *testing.B) {
	state := NewHistoryState(50000)
	for i := 0; i < 50; i++ {
		state.Append(&UserCell{Text: fmt.Sprintf("Question %d", i)})
		state.Append(&AssistantCell{Text: fmt.Sprintf("## Answer %d\n\n- item one\n- item two with `code`", i)})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = state.RenderLinesAt(68)
	}
}

func BenchmarkRefreshViewport100Cells(b *testing.B) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	m.resize(80, 24)
	m.showWelcome = false
	for i := 0; i < 50; i++ {
		m.historyState.Append(&UserCell{Text: fmt.Sprintf("Question %d", i)})
		m.historyState.Append(&AssistantCell{Text: fmt.Sprintf("Answer %d with **markdown** and `code`.", i)})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.refreshViewport()
	}
}

func BenchmarkAssistantStreamingMarkdown20KB(b *testing.B) {
	chunk := "A paragraph with **bold text**, `inline code`, and [a link](https://example.com).\n"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state := NewHistoryState(50000)
		for j := 0; j < 250; j++ {
			state.AppendAssistantDelta(chunk)
			_ = state.RenderLines()
		}
	}
}

func BenchmarkHistoryStateRaw100Cells(b *testing.B) {
	state := NewHistoryState(50000)
	for i := 0; i < 100; i++ {
		state.Append(&AssistantCell{Text: fmt.Sprintf("raw transcript line %d", i)})
	}
	_ = state.Raw()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = state.Raw()
	}
}

func BenchmarkHistoryStateRenderContentActiveMarkdown20KB(b *testing.B) {
	state := NewHistoryState(50000)
	for i := 0; i < 100; i++ {
		state.Append(&AssistantCell{Text: fmt.Sprintf("Committed response %d with **bold** and `code`.", i)})
	}
	state.AppendAssistantDelta(strings.Repeat("A paragraph with **bold text**, `inline code`, and [a link](https://example.com).\n", 250))
	_ = state.RenderContent()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = state.RenderContent()
	}
}

func BenchmarkHistoryStateRenderJoinedActiveMarkdown20KB(b *testing.B) {
	state := NewHistoryState(50000)
	for i := 0; i < 100; i++ {
		state.Append(&AssistantCell{Text: fmt.Sprintf("Committed response %d with **bold** and `code`.", i)})
	}
	state.AppendAssistantDelta(strings.Repeat("A paragraph with **bold text**, `inline code`, and [a link](https://example.com).\n", 250))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = strings.Join(state.RenderLines(), "\n")
	}
}

func BenchmarkRefreshViewportStreamingLongHistory(b *testing.B) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	m.resize(100, 30)
	m.showWelcome = false
	m.busy = true
	m.followTail = true
	for i := 0; i < 500; i++ {
		m.historyState.Append(&UserCell{Text: fmt.Sprintf("Question %d with enough text to represent a realistic long session", i)})
		m.historyState.Append(&AssistantCell{Text: fmt.Sprintf("Answer %d with **markdown**, `code`, and a second line.\nMore detail here.", i)})
	}
	m.historyState.AppendAssistantDelta(strings.Repeat("streaming **tail** with `code` and details\n", 250))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.refreshViewport()
	}
}

func BenchmarkAssistantStreamingTailContent20KB(b *testing.B) {
	chunk := "A paragraph with **bold text**, `inline code`, and [a link](https://example.com).\n"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state := NewHistoryState(50000)
		state.SetWidth(100)
		for j := 0; j < 250; j++ {
			state.AppendAssistantDelta(chunk)
			_, _ = state.RenderTailContent(24)
		}
	}
}

func BenchmarkApplyTurnTextDeltaLongHistory(b *testing.B) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	for i := 0; i < 500; i++ {
		m.historyState.Append(&UserCell{Text: fmt.Sprintf("question %d", i)})
		m.historyState.Append(&AssistantCell{Text: fmt.Sprintf("answer %d", i)})
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.applyTurnEvent(applicationturn.Event{Kind: applicationturn.EventTextDelta, Text: "x"})
	}
}

func BenchmarkRefreshViewportScrolledLongHistory(b *testing.B) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	m.resize(100, 30)
	m.showWelcome = false
	for i := 0; i < 500; i++ {
		m.historyState.Append(&UserCell{Text: fmt.Sprintf("Question %d with enough text for scrolling", i)})
		m.historyState.Append(&AssistantCell{Text: fmt.Sprintf("Answer %d with **markdown** and `code`.\nMore detail.", i)})
	}
	m.refreshViewport()
	m.followTail = false
	m.viewport.SetYOffset(maxInt(0, m.viewport.TotalLineCount()/2))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.refreshViewport()
	}
}

func BenchmarkViewBusyLongHistory(b *testing.B) {
	m := newBubbleModel(context.Background(), nil, nil, nil, nil, newPermissionBridge(), "/tmp/proton")
	m.resize(100, 30)
	m.busy = true
	m.followTail = true
	for i := 0; i < 500; i++ {
		m.historyState.Append(&AssistantCell{Text: fmt.Sprintf("Answer %d with **markdown** and `code`.", i)})
	}
	m.refreshViewport()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.View().Content
	}
}
