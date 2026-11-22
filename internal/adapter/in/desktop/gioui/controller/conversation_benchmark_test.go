//go:build desktop || desktop_gio

package controller

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

func BenchmarkApplyStagedHistoryPruneOverflow(b *testing.B) {
	const count = 4096
	events := make([]desktopstate.Event, count)
	text := strings.Repeat("x", 8<<10)
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
	b.SetBytes(int64(count) * int64(len(text)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		controller := benchmarkHistoryApplicationController()
		controller.state.Sessions[0].Timeline = nil
		controller.timelineBytes["session-a"] = 0
		controller.mu.Lock()
		controller.applyStagedHistoryEventsLocked("session-a", events)
		controller.mu.Unlock()
	}
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

// BenchmarkFlushMessageStreamScaling guards the O(1) stream-flush path: the cost
// of a flush must stay flat as the retained timeline grows, because the cached
// item index is reused instead of rescanning the whole timeline every frame.
func BenchmarkFlushMessageStreamScaling(b *testing.B) {
	for _, items := range []int{16, 256, 4096, maxSessionTimelineItems} {
		b.Run("items-"+strconv.Itoa(items), func(b *testing.B) {
			controller := newTestController()
			controller.state.ActiveSessionID = "session-1"
			timeline := make([]desktopstate.TimelineItem, items)
			for index := range timeline {
				timeline[index] = desktopstate.TimelineItem{
					ID: "message-" + strconv.Itoa(index), Kind: desktopstate.TimelineAssistant, Text: strings.Repeat("x", 32),
				}
			}
			controller.state.Sessions[0].Timeline = timeline
			controller.timelineBytes["session-1"] = timelineSize(timeline)

			key := messageStreamKey{sessionID: "session-1", kind: "agent_message_chunk"}
			buffer := &messageStreamBuffer{
				sessionID: "session-1", itemID: "message-" + strconv.Itoa(items-1), kind: desktopstate.TimelineAssistant, index: -1,
			}
			buffer.text.WriteString("streamed text")
			controller.messageStreamBuffers[key] = buffer

			// Warm the cached index so the measured loop exercises the O(1) path.
			controller.mu.Lock()
			controller.flushMessageStreamLocked(buffer)
			controller.mu.Unlock()

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				controller.mu.Lock()
				controller.flushMessageStreamLocked(buffer)
				controller.mu.Unlock()
			}
		})
	}
}

// BenchmarkUpsertTimelineScaling guards the tool-update path: merging an existing
// tail item must not rescan the whole timeline twice as it grows.
func BenchmarkUpsertTimelineScaling(b *testing.B) {
	for _, items := range []int{16, 256, 4096, maxSessionTimelineItems} {
		b.Run("items-"+strconv.Itoa(items), func(b *testing.B) {
			controller := newTestController()
			controller.state.ActiveSessionID = "session-1"
			timeline := make([]desktopstate.TimelineItem, 0, items)
			for index := 0; index < items-1; index++ {
				timeline = append(timeline, desktopstate.TimelineItem{
					ID: "message-" + strconv.Itoa(index), Kind: desktopstate.TimelineAssistant, Text: strings.Repeat("x", 32),
				})
			}
			timeline = append(timeline, desktopstate.TimelineItem{ID: "tool-live", Kind: desktopstate.TimelineTool, Title: "read", Status: "in_progress"})
			controller.state.Sessions[0].Timeline = timeline
			controller.timelineBytes["session-1"] = timelineSize(timeline)

			event := desktopstate.Event{
				Kind: desktopstate.EventTimelineUpserted, SessionID: "session-1",
				Item: desktopstate.TimelineItem{ID: "tool-live", Kind: desktopstate.TimelineTool, Title: "read", Status: "completed"},
			}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				controller.mu.Lock()
				controller.applyTimelineEventLocked(event)
				controller.mu.Unlock()
			}
		})
	}
}
