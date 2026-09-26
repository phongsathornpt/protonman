package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/memory"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/engine/turn"
)

func TestMemoryContextActivityAppearsInTranscriptWithoutMemoryContents(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.applyTurnEvent(turn.Event{
		Kind: turn.EventMemoryActivity,
		MemoryActivity: memory.Activity{
			Kind: memory.ActivityContextIncluded, WorkspaceEntries: 1, GlobalEntries: 1,
		},
	})

	got := plainTranscript(m)
	if !strings.Contains(got, "Memory · Included 2 saved memory items in model request (workspace 1, global 1)") {
		t.Fatalf("transcript missing memory context activity: %q", got)
	}
	if strings.Contains(got, "stored memory contents") {
		t.Fatalf("transcript exposed memory contents: %q", got)
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
