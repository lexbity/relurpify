package analysis

import (
	"sync"

	"codeburg.org/lexbit/relurpify/agents/goalcon/types"
)

// GoalCache is a simple LRU-like cache for goal classifications.
// It avoids re-classifying identical task instructions.
type GoalCache struct {
	mu      sync.RWMutex
	cache   map[string]*types.GoalCondition
	maxSize int
}

// NewGoalCache creates a new cache with a maximum size.
func NewGoalCache(maxSize int) *GoalCache {
	if maxSize <= 0 {
		maxSize = 256 // default
	}
	return &GoalCache{
		cache:   make(map[string]*types.GoalCondition),
		maxSize: maxSize,
	}
}

// Get retrieves a cached goal, or nil if not found.
func (c *GoalCache) Get(instruction string) *types.GoalCondition {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if goal, ok := c.cache[instruction]; ok {
		return goal
	}
	return nil
}

// Set stores a goal in the cache.
func (c *GoalCache) Set(instruction string, goal *types.GoalCondition) {
	if c == nil || goal == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple size management: clear if at capacity
	if len(c.cache) >= c.maxSize && c.cache[instruction] == nil {
		c.cache = make(map[string]*types.GoalCondition)
	}

	c.cache[instruction] = goal
}

// Clear empties the cache.
func (c *GoalCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*types.GoalCondition)
}

// Size returns the number of cached entries.
func (c *GoalCache) Size() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}
