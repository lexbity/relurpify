package lsp

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProxy_Defaults(t *testing.T) {
	p := NewProxy(0, 0)
	require.NotNil(t, p)
	assert.Equal(t, time.Minute, p.ttl)
	assert.Equal(t, 512, p.maxSize)
	assert.NotNil(t, p.cache)
	assert.NotNil(t, p.inflight)
	assert.NotNil(t, p.ll)
}

func TestNewProxy_CustomValues(t *testing.T) {
	p := NewProxy(5*time.Second, 100)
	assert.Equal(t, 5*time.Second, p.ttl)
	assert.Equal(t, 100, p.maxSize)
}

func TestCached_Hit(t *testing.T) {
	p := NewProxy(time.Minute, 10)
	var callCount int32
	fetch := func() (any, error) {
		atomic.AddInt32(&callCount, 1)
		return "value", nil
	}

	v1, err := p.cached("k", fetch)
	require.NoError(t, err)
	assert.Equal(t, "value", v1)

	v2, err := p.cached("k", fetch)
	require.NoError(t, err)
	assert.Equal(t, "value", v2)

	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "fetch called once")
}

func TestCached_Miss(t *testing.T) {
	p := NewProxy(time.Minute, 10)
	v, err := p.cached("k", func() (any, error) {
		return "val", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "val", v)
}

func TestCached_PropagatesError(t *testing.T) {
	p := NewProxy(time.Minute, 10)
	expected := errors.New("fetch failed")
	_, err := p.cached("k", func() (any, error) {
		return nil, expected
	})
	require.ErrorIs(t, err, expected)
}

func TestCached_AC5_SingleFlight(t *testing.T) {
	p := NewProxy(time.Minute, 10)
	var callCount int32
	var wg sync.WaitGroup

	fetch := func() (any, error) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(50 * time.Millisecond)
		return "deduped", nil
	}

	const n = 10
	results := make([]any, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = p.cached("cold-key", fetch)
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "fetch must run exactly once for N concurrent cold reads")
	for i := 0; i < n; i++ {
		require.NoError(t, errs[i])
		assert.Equal(t, "deduped", results[i])
	}
}

func TestCached_EvictsLRUWhenFull(t *testing.T) {
	p := NewProxy(time.Minute, 3)
	for _, k := range []string{"a", "b", "c"} {
		v, err := p.cached(k, func() (any, error) { return k, nil })
		require.NoError(t, err)
		assert.Equal(t, k, v)
	}

	v, err := p.cached("d", func() (any, error) { return "d", nil })
	require.NoError(t, err)
	assert.Equal(t, "d", v)

	assert.Len(t, p.cache, 3, "cache size must not exceed maxSize")

	v, err = p.cached("a", func() (any, error) { return "a", nil })
	require.NoError(t, err)
	assert.Equal(t, "a", v, "evicted entry must be refetched")
}

func TestCached_UsageUpdatesLRUOrder(t *testing.T) {
	p := NewProxy(time.Minute, 3)
	for _, k := range []string{"a", "b", "c"} {
		_, _ = p.cached(k, func() (any, error) { return k, nil })
	}

	_, _ = p.cached("a", func() (any, error) { return "a", nil })

	_, _ = p.cached("d", func() (any, error) { return "d", nil })

	assert.Len(t, p.cache, 3)

	v, err := p.cached("b", func() (any, error) { return "b", nil })
	require.NoError(t, err)
	assert.Equal(t, "b", v, "'b' is LRU and should have been evicted")
}

func TestCached_ExpiredEntry(t *testing.T) {
	p := NewProxy(10*time.Millisecond, 10)
	_, _ = p.cached("k", func() (any, error) { return "v1", nil })

	time.Sleep(20 * time.Millisecond)

	v, err := p.cached("k", func() (any, error) { return "v2", nil })
	require.NoError(t, err)
	assert.Equal(t, "v2", v, "expired entry should be refetched")
}

func TestCached_SingleFlightErrorPropagatesToAll(t *testing.T) {
	p := NewProxy(time.Minute, 10)
	expected := errors.New("transient failure")
	var callCount int32
	var wg sync.WaitGroup

	fetch := func() (any, error) {
		atomic.AddInt32(&callCount, 1)
		time.Sleep(30 * time.Millisecond)
		return nil, expected
	}

	const n = 5
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = p.cached("err-key", fetch)
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount), "fetch must run once even when it errors")
	for i := 0; i < n; i++ {
		require.ErrorIs(t, errs[i], expected)
	}
}
