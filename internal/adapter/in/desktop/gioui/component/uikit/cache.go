//go:build desktop || desktop_gio

package uikit

// BoundedCache is a small FIFO cache with deterministic eviction. Insertion
// order is tracked explicitly so eviction is deterministic; the previous
// per-map `for key := range map { delete }` pattern evicted an arbitrary entry,
// which could drop state that was still on screen.
//
// Eviction shifts the order slice; at the bounded sizes the desktop uses
// (hundreds of entries) that copy is cheaper and far easier to audit than a ring
// buffer.
//
// Not safe for concurrent use: the shell and view code that owns these caches
// run on the single layout goroutine.
type BoundedCache[K comparable, V any] struct {
	items map[K]V
	order []K
	limit int
}

func NewBoundedCache[K comparable, V any](limit int) BoundedCache[K, V] {
	return BoundedCache[K, V]{
		items: make(map[K]V, min(limit, 64)),
		limit: limit,
	}
}

// Get returns the cached value for key.
func (c *BoundedCache[K, V]) Get(key K) (V, bool) {
	value, ok := c.items[key]
	return value, ok
}

// Put inserts or replaces an entry. When the cache is full the oldest entry is
// evicted first and returned so companion maps keyed by the same identity can
// drop their state in the same step. Replacement does not change order: these
// caches are identity-bounded, not LRU.
func (c *BoundedCache[K, V]) Put(key K, value V) (evicted K, evictedValue V, evictedOK bool) {
	if _, exists := c.items[key]; exists {
		c.items[key] = value
		return evicted, evictedValue, false
	}
	if c.limit <= 0 {
		return key, value, true
	}
	if len(c.order) >= c.limit {
		evicted = c.order[0]
		evictedValue = c.items[evicted]
		evictedOK = true
		delete(c.items, evicted)
		copy(c.order, c.order[1:])
		c.order[len(c.order)-1] = key
	} else {
		c.order = append(c.order, key)
	}
	c.items[key] = value
	return evicted, evictedValue, evictedOK
}

// Len reports the number of live entries.
func (c *BoundedCache[K, V]) Len() int {
	return len(c.items)
}

// Reset drops all entries and reclaims order-slice capacity.
func (c *BoundedCache[K, V]) Reset() {
	clear(c.items)
	c.order = c.order[:0]
}
