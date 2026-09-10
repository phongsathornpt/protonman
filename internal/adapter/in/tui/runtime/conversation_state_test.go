package runtime

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/conversation"
)

func TestConversationStateQueueLifecycle(t *testing.T) {
	state := conversationModelState{conversationRetention: conversation.DefaultRetentionPolicy()}
	if !state.enqueue("first") || !state.enqueue("second") {
		t.Fatal("enqueue failed")
	}
	first, ok := state.dequeue()
	if !ok || first != "first" {
		t.Fatalf("first dequeue = %q, %v", first, ok)
	}
	state.clearQueue()
	if len(state.queue) != 0 {
		t.Fatalf("queue length = %d, want 0", len(state.queue))
	}
}

func TestConversationStateDropsOnlyTrailingUser(t *testing.T) {
	state := conversationModelState{messages: []model.Message{{Role: model.RoleAssistant, Content: "a"}, {Role: model.RoleUser, Content: "u"}}}
	if !state.dropTrailingUserMessage() || len(state.messages) != 1 {
		t.Fatalf("messages after drop = %#v", state.messages)
	}
	if state.dropTrailingUserMessage() {
		t.Fatal("dropped non-user trailing message")
	}
}
