package desktop

import "testing"

func TestTimelineEventNormalizesToolLifecycle(t *testing.T) {
	start, ok := TimelineEvent(SessionUpdate{
		SessionID:  "s1",
		Kind:       "tool_call",
		ToolCallID: "call-1",
		Title:      "Read README.md",
		Status:     "in_progress",
	})
	if !ok {
		t.Fatal("tool_call was not normalized")
	}
	if start.Kind != EventTimelineUpserted || start.Item.Kind != TimelineTool {
		t.Fatalf("start event = %#v", start)
	}

	state := State{Sessions: []SessionState{{ID: "s1"}}}
	state = Reduce(state, start)

	finish, ok := TimelineEvent(SessionUpdate{
		SessionID:  "s1",
		Kind:       "tool_call_update",
		ToolCallID: "call-1",
		Status:     "completed",
	})
	if !ok {
		t.Fatal("tool_call_update was not normalized")
	}
	state = Reduce(state, finish)

	got := state.Sessions[0].Timeline
	if len(got) != 1 {
		t.Fatalf("timeline len = %d, want 1", len(got))
	}
	if got[0].Title != "Read README.md" {
		t.Fatalf("title = %q, want preserved title", got[0].Title)
	}
	if got[0].Status != "completed" {
		t.Fatalf("status = %q, want completed", got[0].Status)
	}
}

func TestTimelineEventIgnoresMalformedToolUpdate(t *testing.T) {
	_, ok := TimelineEvent(SessionUpdate{
		SessionID: "s1",
		Kind:      "tool_call_update",
		Status:    "completed",
	})
	if ok {
		t.Fatal("tool update without toolCallId should be ignored")
	}
}

func TestMergeTimelineItemPreservesSparseFields(t *testing.T) {
	previous := TimelineItem{
		Kind:   TimelineTool,
		ID:     "call-1",
		Title:  "Read file",
		Text:   "README.md",
		Status: "in_progress",
	}
	next := TimelineItem{
		Kind:   TimelineTool,
		ID:     "call-1",
		Status: "failed",
	}
	got := MergeTimelineItem(previous, next)
	if got.Title != previous.Title || got.Text != previous.Text {
		t.Fatalf("merge dropped fields: %#v", got)
	}
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
}

func TestMergeTimelineItemConcatenatesMessageChunks(t *testing.T) {
	previous := TimelineItem{Kind: TimelineAssistant, ID: "message-1", Text: "Hello", Streaming: true}
	next := TimelineItem{Kind: TimelineAssistant, ID: "message-1", Text: " world", Streaming: true}
	got := MergeTimelineItem(previous, next)
	if got.Text != "Hello world" {
		t.Fatalf("merged text = %q", got.Text)
	}
	if !got.Streaming {
		t.Fatal("merged message stream was marked complete")
	}
}

func TestMergeTimelineItemDoesNotDuplicateEmptyMessageChunk(t *testing.T) {
	previous := TimelineItem{Kind: TimelineUser, ID: "message-1", Text: "Keep this text"}
	got := MergeTimelineItem(previous, TimelineItem{Kind: TimelineUser, ID: "message-1"})
	if got.Text != previous.Text {
		t.Fatalf("merged text = %q, want %q", got.Text, previous.Text)
	}
}
