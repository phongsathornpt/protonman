package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/memory"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

func TestMemoryContextActivityOmittedFromTranscript(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.applyTurnEvent(turn.Event{
		Kind: turn.EventMemoryActivity,
		MemoryActivity: memory.Activity{
			Kind: memory.ActivityContextIncluded, WorkspaceEntries: 1, GlobalEntries: 1,
		},
	})

	got := plainTranscript(m)
	if strings.Contains(got, "Included") || strings.Contains(got, "memory") || strings.Contains(got, "Memory") {
		t.Fatalf("expected transcript to omit routine memory context activity, got: %q", got)
	}
	if strings.Contains(got, "stored memory contents") {
		t.Fatalf("transcript exposed memory contents: %q", got)
	}

	// Verify durable saves are still surfaced.
	m.applyTurnEvent(turn.Event{
		Kind: turn.EventMemoryActivity,
		MemoryActivity: memory.Activity{
			Kind: memory.ActivityEntriesSaved, WorkspaceEntries: 1,
		},
	})
	gotSaved := plainTranscript(m)
	if !strings.Contains(gotSaved, "Saved 1 memory item") {
		t.Fatalf("expected durable memory save in transcript, got: %q", gotSaved)
	}
}

func TestBackgroundMemoryActivityIsQueuedForTUI(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.ctx = context.Background()
	m.memoryActivities = make(chan memory.Activity, 1)
	m.queueMemoryActivity(memory.Activity{Kind: memory.ActivityEntriesSaved, WorkspaceEntries: 2})

	command := m.nextMemoryActivity()
	raw := command()
	msg, ok := raw.(memoryActivityMsg)
	if !ok {
		t.Fatalf("memory command message = %T, want memoryActivityMsg", raw)
	}
	if msg.activity.Kind != memory.ActivityEntriesSaved || msg.activity.WorkspaceEntries != 2 {
		t.Fatalf("memory activity = %+v", msg.activity)
	}
}
