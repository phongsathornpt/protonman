package todo

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestStoreReplaceRevisionAndClone(t *testing.T) {
	initial := []Item{{ID: "one", Text: "one", Status: StatusPending}}
	store, err := NewStore(initial)
	if err != nil {
		t.Fatal(err)
	}
	initial[0].Text = "mutated"
	if got := store.Snapshot().Items[0].Text; got != "one" {
		t.Fatalf("snapshot = %q", got)
	}

	same, err := store.Replace(context.Background(), store.Snapshot().Items)
	if err != nil {
		t.Fatal(err)
	}
	if same.Revision != 0 {
		t.Fatalf("no-op revision = %d", same.Revision)
	}

	next := []Item{{ID: "one", Text: "one", Status: StatusCompleted}}
	changed, err := store.Replace(context.Background(), next)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Revision != 1 {
		t.Fatalf("changed revision = %d", changed.Revision)
	}
	changed.Items[0].Text = "caller mutation"
	if got := store.Snapshot().Items[0].Text; got != "one" {
		t.Fatalf("store leaked backing slice: %q", got)
	}
}

func TestStoreReplaceHonorsCancellation(t *testing.T) {
	store, err := NewStore([]Item{{ID: "one", Text: "one", Status: StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.Replace(ctx, []Item{{ID: "two", Text: "two", Status: StatusPending}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if got := store.Snapshot(); got.Revision != 0 || got.Items[0].ID != "one" {
		t.Fatalf("store mutated: %#v", got)
	}
}

func TestStoreConcurrentSnapshotAndReplace(t *testing.T) {
	store, err := NewStore([]Item{{ID: "one", Text: "one", Status: StatusPending}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				if worker%2 == 0 {
					_ = store.Snapshot()
					continue
				}
				status := StatusPending
				if n%2 == 0 {
					status = StatusInProgress
				}
				_, _ = store.Replace(context.Background(), []Item{{ID: "one", Text: "one", Status: status}})
			}
		}(i)
	}
	wg.Wait()
	if err := ValidateItems(store.Snapshot().Items); err != nil {
		t.Fatal(err)
	}
}

func TestStoreCompareAndReplaceRejectsStaleRevision(t *testing.T) {
	store, err := NewStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndReplace(context.Background(), 0, []Item{{ID: "a", Text: "one", Status: StatusPending}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndReplace(context.Background(), 0, []Item{{ID: "b", Text: "two", Status: StatusPending}}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("error=%v, want revision conflict", err)
	}
	got := store.Snapshot()
	if got.Revision != 1 || len(got.Items) != 1 || got.Items[0].ID != "a" {
		t.Fatalf("snapshot=%#v", got)
	}
}
