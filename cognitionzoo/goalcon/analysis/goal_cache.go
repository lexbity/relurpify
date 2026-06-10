package analysis

import (
	"container/list"
	"sync"

	"codeburg.org/lexbit/relurpify/cognitionzoo/goalcon/types"
)

type goalCacheEntry struct {
	instruction string
	goal        *types.GoalCondition
	element     *list.Element
}

type GoalCache struct {
	mu      sync.RWMutex
	ll      *list.List
	cache   map[string]*goalCacheEntry
	maxSize int
}

func NewGoalCache(maxSize int) *GoalCache {
	if maxSize <= 0 {
		maxSize = 256
	}
	return &GoalCache{
		ll:      list.New(),
		cache:   make(map[string]*goalCacheEntry, maxSize),
		maxSize: maxSize,
	}
}

func (c *GoalCache) Get(instruction string) *types.GoalCondition {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	if entry, ok := c.cache[instruction]; ok {
		c.mu.RUnlock()
		c.mu.Lock()
		c.ll.MoveToFront(entry.element)
		c.mu.Unlock()
		return entry.goal
	}
	c.mu.RUnlock()
	return nil
}

func (c *GoalCache) Set(instruction string, goal *types.GoalCondition) {
	if c == nil || goal == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.cache[instruction]; ok {
		entry.goal = goal
		c.ll.MoveToFront(entry.element)
		return
	}

	if len(c.cache) >= c.maxSize {
		c.evictLocked()
	}

	entry := &goalCacheEntry{instruction: instruction, goal: goal}
	entry.element = c.ll.PushFront(entry)
	c.cache[instruction] = entry
}

func (c *GoalCache) evictLocked() {
	back := c.ll.Back()
	if back == nil {
		return
	}
	entry := back.Value.(*goalCacheEntry)
	c.ll.Remove(back)
	delete(c.cache, entry.instruction)
}

func (c *GoalCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll = list.New()
	c.cache = make(map[string]*goalCacheEntry)
}

func (c *GoalCache) Size() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}
