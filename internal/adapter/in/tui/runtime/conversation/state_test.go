package conversation

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	coreconv "github.com/phongsathornpt/protonman/internal/core/conversation"
)

func TestStateQueueLifecycle(t *testing.T) {
	state := NewState(coreconv.DefaultRetentionPolicy())
	if !state.Enqueue("first") || !state.Enqueue("second") {
		t.Fatal("enqueue failed")
	}
	if state.QueueLen() != 2 {
		t.Fatalf("queue len = %d, want 2", state.QueueLen())
	}
	first, ok := state.Dequeue()
	if !ok || first != "first" {
		t.Fatalf("first dequeue = %q, %v", first, ok)
	}
	state.ClearQueue()
	if state.QueueLen() != 0 {
		t.Fatalf("queue length = %d, want 0", state.QueueLen())
	}
}

func TestStateDropsOnlyTrailingUser(t *testing.T) {
	state := NewState(coreconv.DefaultRetentionPolicy())
	state.SetMessages([]model.Message{
		{Role: model.RoleAssistant, Content: "a"},
		{Role: model.RoleUser, Content: "u"},
	})
	if !state.DropTrailingUserMessage() || len(state.Messages()) != 1 {
		t.Fatalf("messages after drop = %#v", state.Messages())
	}
	if state.DropTrailingUserMessage() {
		t.Fatal("dropped non-user trailing message")
	}
}

func TestStateReset(t *testing.T) {
	state := NewState(coreconv.DefaultRetentionPolicy())
	state.AppendMessages(model.Message{Role: model.RoleUser, Content: "hi"})
	state.Enqueue("queued prompt")
	state.Reset()
	if len(state.Messages()) != 0 || state.QueueLen() != 0 {
		t.Fatalf("state not reset: messages=%d queue=%d", len(state.Messages()), state.QueueLen())
	}
}

func TestQueuePreview(t *testing.T) {
	if got := QueuePreview("short"); got != "short" {
		t.Fatalf("got %q, want 'short'", got)
	}
	long := "this is a very long prompt that exceeds the maximum preview limit"
	got := QueuePreview(long, 10)
	if got != "this is a…" {
		t.Fatalf("got %q, want 'this is a…'", got)
	}
}
