package analysis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"codeburg.org/lexbit/relurpify/cognitionzoo/goalcon/types"
)

func TestNewGoalCache_DefaultSize(t *testing.T) {
	c := NewGoalCache(0)
	require.NotNil(t, c)
	assert.Equal(t, 0, c.Size())
}

func TestGoalCache_GetNil(t *testing.T) {
	var c *GoalCache
	assert.Nil(t, c.Get("key"))
}

func TestGoalCache_SetGet(t *testing.T) {
	c := NewGoalCache(10)
	g := &types.GoalCondition{Predicates: []types.Predicate{"p1"}}
	c.Set("a", g)
	got := c.Get("a")
	require.NotNil(t, got)
	assert.Equal(t, []types.Predicate{"p1"}, got.Predicates)
}

func TestGoalCache_GetMissing(t *testing.T) {
	c := NewGoalCache(10)
	assert.Nil(t, c.Get("missing"))
}

func TestGoalCache_SetNilGoal(t *testing.T) {
	c := NewGoalCache(10)
	c.Set("a", nil)
	assert.Nil(t, c.Get("a"))
	assert.Equal(t, 0, c.Size())
}

func TestGoalCache_EvictsLRUWhenFull(t *testing.T) {
	c := NewGoalCache(3)
	for _, k := range []string{"a", "b", "c"} {
		c.Set(k, &types.GoalCondition{Description: k})
	}
	require.Equal(t, 3, c.Size())

	c.Set("d", &types.GoalCondition{Description: "d"})
	assert.Equal(t, 3, c.Size(), "size should stay at capacity")

	assert.Nil(t, c.Get("a"), "LRU entry 'a' should be evicted")
	assert.NotNil(t, c.Get("b"))
	assert.NotNil(t, c.Get("c"))
	assert.NotNil(t, c.Get("d"))
}

func TestGoalCache_AC4_EvictsLRUNotFlushes(t *testing.T) {
	c := NewGoalCache(3)
	for _, k := range []string{"a", "b", "c"} {
		c.Set(k, &types.GoalCondition{Description: k})
	}

	c.Get("a")

	c.Set("d", &types.GoalCondition{Description: "d"})
	assert.Equal(t, 3, c.Size())

	assert.NotNil(t, c.Get("a"), "'a' was recently used, should survive")
	assert.Nil(t, c.Get("b"), "'b' is LRU, should be evicted")
}

func TestGoalCache_Clear(t *testing.T) {
	c := NewGoalCache(10)
	c.Set("a", &types.GoalCondition{Description: "a"})
	c.Set("b", &types.GoalCondition{Description: "b"})
	assert.Equal(t, 2, c.Size())

	c.Clear()
	assert.Equal(t, 0, c.Size())
	assert.Nil(t, c.Get("a"))
}

func TestGoalCache_UpdateExisting(t *testing.T) {
	c := NewGoalCache(3)
	c.Set("a", &types.GoalCondition{Description: "old"})
	c.Set("a", &types.GoalCondition{Description: "new"})
	assert.Equal(t, 1, c.Size())
	got := c.Get("a")
	require.NotNil(t, got)
	assert.Equal(t, "new", got.Description)
}

func TestGoalCache_Size(t *testing.T) {
	c := NewGoalCache(5)
	assert.Equal(t, 0, c.Size())
	c.Set("a", &types.GoalCondition{Description: "a"})
	assert.Equal(t, 1, c.Size())
}

func TestGoalCache_NilSize(t *testing.T) {
	var c *GoalCache
	assert.Equal(t, 0, c.Size())
}
