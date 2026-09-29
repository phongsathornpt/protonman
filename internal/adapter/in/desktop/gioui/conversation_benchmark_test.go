//go:build desktop || desktop_gio

package gioui

import (
	"encoding/json"
	"strconv"
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

func BenchmarkApplyStagedSessionHistory(b *testing.B) {
	const count = maxSessionTimelineItems
	events := make([]desktopstate.Event, count)
	text := strings.Repeat("x", 128)
	for index := range events {
		events[index] = desktopstate.Event{
			Kind:      desktopstate.EventTimelineAppended,
			SessionID: "session-a",
			Item: desktopstate.TimelineItem{
				Kind: desktopstate.TimelineAssistant,
				ID:   "message-" + strconv.Itoa(index),
				Text: text,
			},
		}
	}

	b.Run("incremental", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			controller := benchmarkHistoryApplicationController()
			controller.mu.Lock()
			for _, event := range events {
				controller.applyTimelineEventLocked(event)
			}
			controller.mu.Unlock()
		}
	})
	b.Run("batched", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			controller := benchmarkHistoryApplicationController()
			controller.mu.Lock()
			controller.applyStagedHistoryEventsLocked("session-a", events)
			controller.mu.Unlock()
		}
	})
}

func benchmarkHistoryApplicationController() *controller {
	sessions := make([]desktopstate.SessionState, 64)
	for index := range sessions {
		sessions[index].ID = "session-" + strconv.Itoa(index)
	}
	sessions[0].ID = "session-a"
	return &controller{
		state:         desktopstate.State{ActiveSessionID: sessions[0].ID, Sessions: sessions},
		timelineBytes: map[string]int{"session-a": 0},
	}
}
