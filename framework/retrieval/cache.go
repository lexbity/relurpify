package retrieval

import (
	"container/list"
	"sync"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

const (
	defaultExactCacheEntries = 128
	defaultExactCacheTTL     = 5 * time.Minute
)

// CacheConfig controls the in-process exact-query cache.
type CacheConfig struct {
	MaxEntries int
	TTL        time.Duration
}

type exactCache struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	items      map[string]*list.Element
	order      *list.List
}

type cacheEntry struct {
	key       string
	blocks    []core.ContentBlock
	event     RetrievalEvent
	expiresAt time.Time
}

func newExactCache(cfg CacheConfig) *exactCache {
	maxEntries := cfg.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultExactCacheEntries
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = defaultExactCacheTTL
	}
	return &exactCache{
		maxEntries: maxEntries,
		ttl:        ttl,
		items:      make(map[string]*list.Element, maxEntries),
		order:      list.New(),
	}
}

func (c *exactCache) get(key string, now time.Time) ([]core.ContentBlock, RetrievalEvent, bool) {
	if c == nil {
		return nil, RetrievalEvent{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return nil, RetrievalEvent{}, false
	}
	entry := elem.Value.(*cacheEntry)
	if now.After(entry.expiresAt) {
		c.removeElement(elem)
		return nil, RetrievalEvent{}, false
	}
	c.order.MoveToFront(elem)
	return cloneBlocks(entry.blocks), entry.event, true
}

func (c *exactCache) set(key string, blocks []core.ContentBlock, event RetrievalEvent, now time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.blocks = cloneBlocks(blocks)
		entry.event = event
		entry.expiresAt = now.Add(c.ttl)
		c.order.MoveToFront(elem)
		return
	}

	entry := &cacheEntry{
		key:       key,
		blocks:    cloneBlocks(blocks),
		event:     event,
		expiresAt: now.Add(c.ttl),
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
	for len(c.items) > c.maxEntries {
		c.removeElement(c.order.Back())
	}
}

func (c *exactCache) removeElement(elem *list.Element) {
	if c == nil || elem == nil {
		return
	}
	c.order.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.key)
}

func cloneBlocks(blocks []core.ContentBlock) []core.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]core.ContentBlock, len(blocks))
	copy(out, blocks)
	return out
}
