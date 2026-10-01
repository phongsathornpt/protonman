//go:build desktop || desktop_gio

package conversation

import "testing"

func TestDraftsReplaceAndEvictOldestSession(t *testing.T) {
	component := New(2)
	component.SaveDraft("one", "first")
	component.SaveDraft("two", "second")
	component.SaveDraft("one", "updated")
	component.SaveDraft("three", "third")
	if got := component.TakeDraft("one"); got != "updated" {
		t.Fatalf("draft one = %q, want updated", got)
	}
	if got := component.TakeDraft("two"); got != "" {
		t.Fatalf("oldest draft two = %q, want evicted", got)
	}
	if got := component.TakeDraft("three"); got != "third" {
		t.Fatalf("draft three = %q, want retained", got)
	}
	mapCount, orderCount := component.DraftCounts()
	if mapCount != 0 || orderCount != 0 {
		t.Fatalf("draft cache counts = (%d, %d), want empty", mapCount, orderCount)
	}
}

func TestWidgetStateIsOwnedPerConversationComponent(t *testing.T) {
	first, second := New(2), New(2)
	first.Editor().SetText("draft")
	first.Timeline().ScrollToEnd = false
	if second.Editor().Text() != "" || !second.Timeline().ScrollToEnd {
		t.Fatal("separate conversation components shared widget state")
	}
	if first.MentionButton("file.go") == second.MentionButton("file.go") {
		t.Fatal("separate conversation components shared mention buttons")
	}
}
