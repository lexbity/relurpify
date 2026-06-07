package authorization

import (
	"sync"
	"testing"
)

func TestGlobCacheHitReturnsSameRegexp(t *testing.T) {
	c := newCompiledGlobCache(256)
	r1, err := c.get(`^foo.*bar$`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r2, err := c.get(`^foo.*bar$`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r1 != r2 {
		t.Fatal("expected same *regexp.Regexp pointer for cache hit")
	}
}

func TestGlobCacheEvictsAtCapacity(t *testing.T) {
	c := newCompiledGlobCache(4)
	// Insert 5 patterns; the first should be evicted.
	for i := 0; i < 5; i++ {
		_, err := c.get(`^pattern_` + string(rune('a'+i)) + `$`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if c.len() > 4 {
		t.Fatalf("expected cache size <= 4, got %d", c.len())
	}
}

func TestGlobCacheInvalidPatternReturnsError(t *testing.T) {
	c := newCompiledGlobCache(256)
	_, err := c.get(`[invalid`)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
}

func TestGlobCacheConcurrentAccess(t *testing.T) {
	c := newCompiledGlobCache(256)
	var wg sync.WaitGroup
	patterns := []string{`^foo$`, `^bar$`, `^baz$`, `^qux$`, `^quux$`}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, p := range patterns {
				_, err := c.get(p)
				if err != nil {
					t.Errorf("unexpected error for pattern %q: %v", p, err)
				}
			}
		}()
	}
	wg.Wait()
	if c.len() != len(patterns) {
		t.Fatalf("expected cache size %d, got %d", len(patterns), c.len())
	}
}

func TestGlobCacheNilIsNoop(t *testing.T) {
	var c *compiledGlobCache
	r, err := c.get(`^test$`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.MatchString("test") {
		t.Fatal("expected regex to match 'test'")
	}
	// nil cache should not panic
	if c.len() != 0 {
		t.Fatal("expected len=0 for nil cache")
	}
}

func TestGlobCacheLRUOrder(t *testing.T) {
	c := newCompiledGlobCache(3)
	// Insert A, B, C
	_, _ = c.get(`^A$`)
	_, _ = c.get(`^B$`)
	_, _ = c.get(`^C$`)

	// Access A (now LRU order: C, B, A)
	_, _ = c.get(`^A$`)

	// Insert D (should evict B, the least recently used)
	_, _ = c.get(`^D$`)

	// B should be evicted
	_, errB := c.get(`^B$`)
	// This creates a NEW entry for B, so it must compile successfully.
	if errB != nil {
		t.Fatalf("unexpected error for B: %v", errB)
	}

	// A should still be cached (hit)
	rA1, _ := c.get(`^A$`)
	rA2, _ := c.get(`^A$`)
	if rA1 != rA2 {
		t.Fatal("A should still be cached")
	}
}

func TestGlobCacheCustomMaxSize(t *testing.T) {
	c := newCompiledGlobCache(0)
	if c.maxSize != 256 {
		t.Fatalf("expected default maxSize 256, got %d", c.maxSize)
	}
}
