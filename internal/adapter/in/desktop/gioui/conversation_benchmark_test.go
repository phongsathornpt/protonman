//go:build desktop || desktop_gio

package gioui

import (
	"encoding/json"
	"strings"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

var sessionUpdateTextBenchmarkSink string

func BenchmarkSessionUpdateTextArrayChunk(b *testing.B) {
	chunk := strings.Repeat("x", 256)
	content, err := json.Marshal([]map[string]string{{"type": "text", "text": chunk}})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		sessionUpdateTextBenchmarkSink = sessionUpdateText(content)
	}
}

func BenchmarkSessionUpdateTextObjectChunk(b *testing.B) {
	chunk := strings.Repeat("x", 256)
	content, err := json.Marshal(map[string]string{"type": "text", "text": chunk})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		sessionUpdateTextBenchmarkSink = sessionUpdateText(content)
	}
}

func BenchmarkAppendAssistantStreamChunks(b *testing.B) {
	const (
		chunks    = 512
		chunkSize = 256
	)
	chunk := strings.Repeat("x", chunkSize)
	b.SetBytes(chunks * chunkSize)
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		controller := newTestController()
		for range chunks {
			controller.mu.Lock()
			event, ok := controller.eventsForSessionUpdateLocked(sessionUpdatePayload{SessionID: "session-1"}, desktopstate.SessionUpdate{
				SessionID: "session-1",
				Kind:      "agent_message_chunk",
				Text:      chunk,
			})
			if ok {
				controller.appendMessageChunkLocked("session-1", "agent_message_chunk", event)
			}
			controller.mu.Unlock()
		}
		controller.mu.Lock()
		controller.clearMessageStreamsLocked("session-1")
		controller.mu.Unlock()
	}
}
