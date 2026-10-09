package bounded

import (
	"sync"
	"testing"
	"time"
)

func TestCacheEnforcesCap(t *testing.T) {
	c := NewCache[string, int](4, 0, nil)
	for i := 0; i < 14; i++ {
		c.Put(string(rune('a'+i)), i)
	}
	if c.Len() != 4 {
		t.Fatalf("Len() = %d, want 4", c.Len())
	}
	if c.Evicted() != 10 {
		t.Fatalf("Evicted() = %d, want 10", c.Evicted())
	}
	// Oldest entries were evicted; the newest four survive.
	for i := 0; i < 10; i++ {
		if _, ok := c.Get(string(rune('a' + i))); ok {
			t.Fatalf("key %d should have been evicted", i)
		}
	}
	for i := 10; i < 14; i++ {
		if v, ok := c.Get(string(rune('a' + i))); !ok || v != i {
			t.Fatalf("key %d missing or wrong value %d", i, v)
		}
	}
}

func TestCacheTTLLazyExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	c := NewCache[string, int](8, time.Minute, nil)
	c.SetClock(func() time.Time { return now })
	c.Put("k", 1)
	if _, ok := c.Get("k"); !ok {
		t.Fatal("fresh entry missing")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expired entry still readable")
	}
	if c.Len() != 0 {
		t.Fatalf("expired entry not deleted, Len() = %d", c.Len())
	}
	if c.Expired() != 1 {
		t.Fatalf("Expired() = %d, want 1", c.Expired())
	}
}

func TestCacheSweep(t *testing.T) {
	now := time.Unix(1000, 0)
	c := NewCache[string, int](8, time.Minute, nil)
	c.SetClock(func() time.Time { return now })
	c.Put("a", 1)
	c.Put("b", 2)
	now = now.Add(2 * time.Minute)
	c.Put("c", 3)
	// a and b expired without access; sweep must remove both, keep c.
	if removed := c.Sweep(); removed != 2 {
		t.Fatalf("Sweep() = %d, want 2", removed)
	}
	if c.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", c.Len())
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("fresh entry swept")
	}
	if c.Sweep() != 0 {
		t.Fatal("Sweep must be idempotent")
	}
}

func TestCacheOnEvictHook(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	c := NewCache[string, int](2, 0, func(k string, v int) {
		mu.Lock()
		seen[k] = v
		mu.Unlock()
	})
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // evicts "a"
	mu.Lock()
	v, ok := seen["a"]
	mu.Unlock()
	if !ok || v != 1 {
		t.Fatalf("onEvict not fired for evicted entry: %+v", seen)
	}
}

func TestCachePutRefreshesLRU(t *testing.T) {
	c := NewCache[string, int](2, 0, nil)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("a", 10) // refresh "a": "b" is now the oldest
	c.Put("c", 3)  // evicts "b"
	if _, ok := c.Get("b"); ok {
		t.Fatal("expected b evicted after a was refreshed")
	}
	if v, ok := c.Get("a"); !ok || v != 10 {
		t.Fatal("refreshed value lost")
	}
}

func TestCacheZeroValueSafety(t *testing.T) {
	// A zero-cap cache accepts no entries and never grows.
	c := NewCache[string, int](0, 0, nil)
	c.Put("k", 1)
	if c.Len() != 0 {
		t.Fatalf("zero-cap cache grew to %d", c.Len())
	}
	if _, ok := c.Get("k"); ok {
		t.Fatal("zero-cap cache returned a value")
	}
}

func TestCacheConcurrentPutGet(t *testing.T) {
	c := NewCache[int, int](64, 0, nil)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				key := (g*500 + i) % 100
				c.Put(key, i)
				c.Get(key)
				if i%50 == 0 {
					c.Sweep()
				}
			}
		}(g)
	}
	wg.Wait()
	if c.Len() > 64 {
		t.Fatalf("cap exceeded: %d", c.Len())
	}
}

func TestRingOverwritesOldest(t *testing.T) {
	r := NewRing[int](3)
	for i := 1; i <= 5; i++ {
		r.Append(i)
	}
	if r.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", r.Len())
	}
	got := r.Snapshot()
	want := []int{3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Snapshot() = %v, want %v", got, want)
		}
	}
}

func TestRingPartialFill(t *testing.T) {
	r := NewRing[string](4)
	r.Append("a")
	r.Append("b")
	got := r.Snapshot()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("Snapshot() = %v, want [a b]", got)
	}
}
