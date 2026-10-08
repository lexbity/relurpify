// Package bounded provides behavioral primitives that enforce the
// "bounded everything" discipline: every cache/map/queue has a cap, an
// eviction policy, and an eviction counter — or it does not exist.
//
// This package is behavior-bearing (not a type-only bucket): its home under
// governance keeps it importable by every domain that needs bounds without
// introducing a new shared dependency.
package bounded

import (
	"sync"
	"sync/atomic"
	"time"
)

// Cache is a bounded map with optional per-entry TTL and insert-order LRU
// eviction. All methods are safe for concurrent use. Evictions and lazy
// expiries increment the exported counters and invoke the onEvict hook when
// one is configured. A cap <= 0 means "no capacity" — every Put evicts the
// entry it inserts; construct with a positive cap. A ttl <= 0 disables
// expiry.
type Cache[K comparable, V any] struct {
	mu      sync.Mutex
	cap     int
	ttl     time.Duration
	entries map[K]*cacheEntry[V]
	order   []K // insert-order LRU: oldest first
	onEvict func(k K, v V)

	evicted atomic.Uint64
	expired atomic.Uint64

	clock func() time.Time
}

type cacheEntry[V any] struct {
	value      V
	insertedAt time.Time
}

// NewCache builds a cache holding at most cap entries with an optional
// per-entry TTL. onEvict (when non-nil) observes capacity evictions, TTL
// expiries, and Sweep removals.
func NewCache[K comparable, V any](cap int, ttl time.Duration, onEvict func(k K, v V)) *Cache[K, V] {
	return &Cache[K, V]{
		cap:     cap,
		ttl:     ttl,
		entries: make(map[K]*cacheEntry[V]),
		onEvict: onEvict,
		clock:   time.Now,
	}
}

// SetClock replaces the time source (deterministic tests). The replacement
// must be installed before first use.
func (c *Cache[K, V]) SetClock(clock func() time.Time) {
	c.mu.Lock()
	c.clock = clock
	c.mu.Unlock()
}

// Get returns the value for k. An expired entry is deleted and counted as a
// lazy expiry; a missing key returns the zero value and false.
func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[k]
	if !ok {
		var zero V
		return zero, false
	}
	if c.expiredLocked(k, entry) {
		var zero V
		return zero, false
	}
	return entry.value, true
}

// expiredLocked deletes the entry when its TTL has elapsed. Caller holds c.mu.
func (c *Cache[K, V]) expiredLocked(k K, entry *cacheEntry[V]) bool {
	if c.ttl <= 0 || entry.insertedAt.IsZero() {
		return false
	}
	if c.clock().Sub(entry.insertedAt) < c.ttl {
		return false
	}
	c.removeLocked(k, entry)
	c.expired.Add(1)
	return true
}

// Put inserts or replaces k. Re-Putting an existing key refreshes its value
// and its LRU position. Inserting past the cap evicts the oldest entry.
func (c *Cache[K, V]) Put(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cap <= 0 {
		if c.onEvict != nil {
			c.onEvict(k, v)
		}
		c.evicted.Add(1)
		return
	}
	if entry, ok := c.entries[k]; ok {
		entry.value = v
		entry.insertedAt = c.clock()
		c.touchLocked(k)
		return
	}
	c.entries[k] = &cacheEntry[V]{value: v, insertedAt: c.clock()}
	c.order = append(c.order, k)
	for len(c.order) > c.cap {
		oldest := c.order[0]
		c.order = c.order[1:]
		if evicted, ok := c.entries[oldest]; ok {
			c.removeLocked(oldest, evicted)
			c.evicted.Add(1)
		}
	}
}

// touchLocked moves k to the newest LRU position. Caller holds c.mu.
func (c *Cache[K, V]) touchLocked(k K) {
	for i, existing := range c.order {
		if existing == k {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, k)
			return
		}
	}
}

// removeLocked deletes the entry and fires the onEvict hook. Caller holds c.mu.
func (c *Cache[K, V]) removeLocked(k K, entry *cacheEntry[V]) {
	delete(c.entries, k)
	if c.onEvict != nil {
		c.onEvict(k, entry.value)
	}
}

// Delete removes k when present.
func (c *Cache[K, V]) Delete(k K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.entries[k]; ok {
		c.removeLocked(k, entry)
		for i, existing := range c.order {
			if existing == k {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
	}
}

// Len reports the number of live entries (including not-yet-swept expired
// ones; lazy expiry removes them on access or via Sweep).
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// Sweep removes every TTL-expired entry in one pass. It is idempotent.
func (c *Cache[K, V]) Sweep() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := 0
	for _, k := range append([]K(nil), c.order...) {
		if entry, ok := c.entries[k]; ok && c.expiredLocked(k, entry) {
			// removeLocked does not touch order; expire from the live map and
			// filter order below.
			removed++
		}
	}
	if removed > 0 {
		live := make([]K, 0, len(c.entries))
		for _, k := range c.order {
			if _, ok := c.entries[k]; ok {
				live = append(live, k)
			}
		}
		c.order = live
	}
	return removed
}

// Evicted reports the capacity-eviction counter.
func (c *Cache[K, V]) Evicted() uint64 { return c.evicted.Load() }

// Expired reports the TTL-expiry counter (lazy and swept).
func (c *Cache[K, V]) Expired() uint64 { return c.expired.Load() }

// Reset drops every entry without firing onEvict (policy reload semantics:
// the old entries are invalidated wholesale, not individually released).
func (c *Cache[K, V]) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[K]*cacheEntry[V])
	c.order = nil
}

// Range calls fn for every live (non-expired) entry under the cache lock;
// returning false from fn stops iteration. Mutation inside fn deadlocks —
// collect keys and act after Range returns.
func (c *Cache[K, V]) Range(fn func(k K, v V) bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, entry := range c.entries {
		if c.ttl > 0 && c.clock().Sub(entry.insertedAt) >= c.ttl {
			continue
		}
		if !fn(k, entry.value) {
			return
		}
	}
}
