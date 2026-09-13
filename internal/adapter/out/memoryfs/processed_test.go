package memoryfs

import (
	"context"
	"testing"
)

func TestProcessedRevisionIsMonotonic(t *testing.T) {
	store, err := NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.MarkProcessed(ctx, "session-1", 4); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkProcessed(ctx, "session-1", 2); err != nil {
		t.Fatal(err)
	}
	revision, ok, err := store.ProcessedRevision(ctx, "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || revision != 4 {
		t.Fatalf("ProcessedRevision() = %d, %v, want 4, true", revision, ok)
	}
}

func TestProcessedRevisionRejectsUnsafeSessionID(t *testing.T) {
	store, _ := NewFileStore(t.TempDir())
	if err := store.MarkProcessed(context.Background(), "../escape", 1); err == nil {
		t.Fatal("MarkProcessed() error = nil, want unsafe session id error")
	}
}
