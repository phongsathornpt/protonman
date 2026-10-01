//go:build desktop || desktop_gio

package uikit

import (
	"strconv"
	"testing"
)

type boundedCacheTestValue struct {
	payload string
}

func TestBoundedCachePutGetRoundTrip(t *testing.T) {
	cache := NewBoundedCache[string, boundedCacheTestValue](4)

	cache.Put("a", boundedCacheTestValue{payload: "alpha"})
	cache.Put("b", boundedCacheTestValue{payload: "beta"})

	got, ok := cache.Get("a")
	if !ok || got.payload != "alpha" {
		t.Fatalf("get(a) = %q, %v; want alpha, true", got.payload, ok)
	}
	if cache.Len() != 2 {
		t.Fatalf("len = %d, want 2", cache.Len())
	}
}

func TestBoundedCacheEvictsOldestFirst(t *testing.T) {
	cache := NewBoundedCache[string, boundedCacheTestValue](3)
	for _, key := range []string{"a", "b", "c"} {
		cache.Put(key, boundedCacheTestValue{payload: key})
	}

	evictedKey, evictedValue, ok := cache.Put("d", boundedCacheTestValue{payload: "d"})
	if !ok || evictedKey != "a" || evictedValue.payload != "a" {
		t.Fatalf("evicted = %q/%q/%v, want a/a/true", evictedKey, evictedValue.payload, ok)
	}
	if _, ok := cache.Get("a"); ok {
		t.Fatal("oldest entry must be evicted")
	}
	for _, key := range []string{"b", "c", "d"} {
		if _, ok := cache.Get(key); !ok {
			t.Fatalf("entry %q must survive", key)
		}
	}
	if cache.Len() != 3 {
		t.Fatalf("len = %d, want 3", cache.Len())
	}
}

func TestBoundedCacheReplaceDoesNotChangeOrder(t *testing.T) {
	cache := NewBoundedCache[string, boundedCacheTestValue](3)
	cache.Put("a", boundedCacheTestValue{payload: "a1"})
	cache.Put("b", boundedCacheTestValue{payload: "b1"})
	cache.Put("c", boundedCacheTestValue{payload: "c1"})

	cache.Put("a", boundedCacheTestValue{payload: "a2"})
	if _, _, ok := cache.Put("d", boundedCacheTestValue{}); !ok {
		t.Fatal("inserting into a full cache must evict")
	}
	// "a" was replaced, not re-inserted, so it stays oldest and is evicted.
	if _, ok := cache.Get("a"); ok {
		t.Fatal("replaced entry must keep its original insertion order")
	}
	if _, ok := cache.Get("b"); !ok {
		t.Fatal("b must survive when a is evicted")
	}
}

func TestBoundedCacheEvictionAcrossWraparound(t *testing.T) {
	const limit = 4
	cache := NewBoundedCache[string, boundedCacheTestValue](limit)

	// Fill, then churn far past the limit so the cursor wraps and compaction
	// kicks in repeatedly.
	for i := range limit * 6 {
		key := "key-" + strconv.Itoa(i)
		cache.Put(key, boundedCacheTestValue{payload: key})
	}
	if cache.Len() != limit {
		t.Fatalf("len = %d, want %d", cache.Len(), limit)
	}
	// The most recent `limit` keys must remain.
	for i := limit * 5; i < limit*6; i++ {
		key := "key-" + strconv.Itoa(i)
		if _, ok := cache.Get(key); !ok {
			t.Fatalf("recent entry %q must survive churn", key)
		}
	}
	for i := range limit * 5 {
		key := "key-" + strconv.Itoa(i)
		if _, ok := cache.Get(key); ok {
			t.Fatalf("stale entry %q must be evicted", key)
		}
	}
}

func TestBoundedCacheReset(t *testing.T) {
	cache := NewBoundedCache[string, boundedCacheTestValue](2)
	cache.Put("a", boundedCacheTestValue{})
	cache.Put("b", boundedCacheTestValue{})

	cache.Reset()

	if cache.Len() != 0 {
		t.Fatalf("len after reset = %d, want 0", cache.Len())
	}
	if _, ok := cache.Get("a"); ok {
		t.Fatal("entries must be gone after reset")
	}
	// The cache must keep working after reset.
	cache.Put("c", boundedCacheTestValue{payload: "c"})
	if got, ok := cache.Get("c"); !ok || got.payload != "c" {
		t.Fatalf("get(c) after reset = %q, %v", got.payload, ok)
	}
}

func TestBoundedCacheZeroLimitEvictsImmediately(t *testing.T) {
	cache := NewBoundedCache[string, boundedCacheTestValue](0)

	evictedKey, _, ok := cache.Put("a", boundedCacheTestValue{})
	if !ok || evictedKey != "a" {
		t.Fatalf("zero-limit cache must refuse entries: evicted=%q ok=%v", evictedKey, ok)
	}
	if cache.Len() != 0 {
		t.Fatalf("len = %d, want 0", cache.Len())
	}
}
