package compiler

import (
	"sync/atomic"
	"time"

	"codeburg.org/lexbit/relurpify/context/knowledge"
	"codeburg.org/lexbit/relurpify/governance/bounded"
)

// Cache bounds (§ cache discipline): every cache has a cap, an eviction
// policy, and counters — or it does not exist.
const (
	// compilationCacheCap bounds in-memory compilation results.
	compilationCacheCap = 256
	// compilationCacheTTL bounds staleness when an invalidation event was
	// dropped by the lossy event bus.
	compilationCacheTTL = 5 * time.Minute
	// invalidatedChunksCap bounds the invalidated-chunk set.
	invalidatedChunksCap = 4096
)

// cacheEntry is the in-memory compilation cache record. Access statistics
// are atomics: the read path never mutates shared state under a read lock.
type cacheEntry struct {
	record     CompilationRecord
	deps       map[knowledge.ChunkID]struct{}
	insertedAt time.Time
	accessedAt atomic.Int64 // unix nanos
	hits       atomic.Int64
}

// compilationCache is the bounded, invalidated compilation cache. Invalidation
// is push (chunk events evict dependents eagerly), plus a bounded invalidated
// set re-checked on every hit, plus TTL and capacity as backstops.
type compilationCache struct {
	entries     *bounded.Cache[CacheKey, *cacheEntry]
	invalidated *bounded.Cache[knowledge.ChunkID, struct{}]

	// invalidationHits counts lookups rejected by the invalidated-set
	// recheck (the event-bus backstop doing its job).
	invalidationHits atomic.Uint64
}

func newCompilationCache(clock func() time.Time) compilationCache {
	return compilationCache{
		entries:     bounded.NewCache[CacheKey, *cacheEntry](compilationCacheCap, compilationCacheTTL, nil),
		invalidated: bounded.NewCache[knowledge.ChunkID, struct{}](invalidatedChunksCap, 0, nil),
	}
}

// setClock installs the compiler's deterministic time source on both caches.
func (cc *compilationCache) setClock(clock func() time.Time) {
	cc.entries.SetClock(clock)
}

// get returns a live entry, applying the invalidated-set recheck on every
// hit. Entries touching an invalidated chunk are dropped and counted.
func (cc *compilationCache) get(key CacheKey) *cacheEntry {
	entry, ok := cc.entries.Get(key)
	if !ok {
		return nil
	}
	if cc.depsInvalidated(entry.deps) {
		cc.entries.Delete(key)
		cc.invalidationHits.Add(1)
		return nil
	}
	entry.hits.Add(1)
	entry.accessedAt.Store(time.Now().UnixNano())
	return entry
}

// put stores a compilation result keyed by its cache identity.
func (cc *compilationCache) put(key CacheKey, entry *cacheEntry) {
	cc.entries.Put(key, entry)
}

// depsInvalidated reports whether any dependency intersects the invalidated set.
func (cc *compilationCache) depsInvalidated(deps map[knowledge.ChunkID]struct{}) bool {
	if len(deps) == 0 {
		return false
	}
	found := false
	cc.invalidated.Range(func(chunkID knowledge.ChunkID, _ struct{}) bool {
		if _, dep := deps[chunkID]; dep {
			found = true
			return false
		}
		return true
	})
	return found
}

// markInvalidated records a chunk as superseded (idempotent set semantics)
// and eagerly evicts every cached compilation that depends on it.
func (cc *compilationCache) markInvalidated(chunkID knowledge.ChunkID) {
	cc.invalidated.Put(chunkID, struct{}{})
	var stale []CacheKey
	cc.entries.Range(func(key CacheKey, entry *cacheEntry) bool {
		if _, dep := entry.deps[chunkID]; dep {
			stale = append(stale, key)
		}
		return true
	})
	for _, key := range stale {
		cc.entries.Delete(key)
	}
}

// reset drops every entry (policy reload semantics: verdicts computed under
// the old policy are void wholesale).
func (cc *compilationCache) reset() {
	cc.entries.Reset()
}
