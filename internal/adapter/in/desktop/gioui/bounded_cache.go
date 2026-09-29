//go:build desktop || desktop_gio

package gioui

// boundedCache is a small FIFO cache for conversation render state. Insertion
// order is tracked explicitly so eviction is deterministic; the previous
// per-map `for key := range map { delete }` pattern evicted an arbitrary
// entry, which could drop state that was still on screen.
//
// Eviction shifts the order slice; at the bounded sizes used by the
// conversation view (hundreds of entries) that copy is cheaper and far easier
// to audit than a ring buffer.
//
// Not safe for concurrent use: the conversation view runs on the single
// layout goroutine.
type boundedCache[K comparable, V any] struct {
	items map[K]V
	order []K
	limit int
}

func newBoundedCache[K comparable, V any](limit int) boundedCache[K, V] {
	return boundedCache[K, V]{
		items: make(map[K]V, min(limit, 64)),
		limit: limit,
	}
}

// get returns the cached value for key.
func (c *boundedCache[K, V]) get(key K) (V, bool) {
	value, ok := c.items[key]
	return value, ok
}

// put inserts or replaces an entry. When the cache is full the oldest entry is
// evicted first and returned so companion maps keyed by the same identity can
// drop their state in the same step. Replacement does not change order: these
// caches are identity-bounded, not LRU.
func (c *boundedCache[K, V]) put(key K, value V) (evicted K, evictedValue V, evictedOK bool) {
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

// len reports the number of live entries.
func (c *boundedCache[K, V]) len() int {
	return len(c.items)
}

// reset drops all entries and reclaims order-slice capacity.
func (c *boundedCache[K, V]) reset() {
	clear(c.items)
	c.order = c.order[:0]
}
