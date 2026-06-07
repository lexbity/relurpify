package authorization

import (
	"container/list"
	"regexp"
	"sync"
)

// globCacheEntry holds a compiled regexp and its position in the LRU list.
type globCacheEntry struct {
	pattern string
	regex   *regexp.Regexp
	element *list.Element
}

// compiledGlobCache is a bounded LRU cache for compiled glob-to-regex
// patterns. It replaces the unbounded process-global sync.Map to prevent
// memory exhaustion from crafted glob patterns.
type compiledGlobCache struct {
	mu      sync.Mutex
	maxSize int
	ll      *list.List
	cache   map[string]*globCacheEntry
}

// newCompiledGlobCache creates a cache with the given capacity.
func newCompiledGlobCache(maxSize int) *compiledGlobCache {
	if maxSize <= 0 {
		maxSize = 256
	}
	return &compiledGlobCache{
		maxSize: maxSize,
		ll:      list.New(),
		cache:   make(map[string]*globCacheEntry, maxSize),
	}
}

// get returns a compiled regexp for the given glob pattern, caching the
// result. If the cache is full, the least recently used entry is evicted.
func (c *compiledGlobCache) get(pattern string) (*regexp.Regexp, error) {
	if c == nil {
		return regexp.Compile(pattern)
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.cache[pattern]; ok {
		c.ll.MoveToFront(entry.element)
		return entry.regex, nil
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	if len(c.cache) >= c.maxSize {
		c.evictLocked()
	}

	entry := &globCacheEntry{pattern: pattern, regex: compiled}
	entry.element = c.ll.PushFront(entry)
	c.cache[pattern] = entry
	return compiled, nil
}

// evictLocked removes the least recently used entry from the cache.
// Must be called with c.mu held.
func (c *compiledGlobCache) evictLocked() {
	back := c.ll.Back()
	if back == nil {
		return
	}
	entry := back.Value.(*globCacheEntry)
	c.ll.Remove(back)
	delete(c.cache, entry.pattern)
}

// len returns the current number of cached entries.
func (c *compiledGlobCache) len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.cache)
}
