package graphdb

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEngineDirtyErrConcurrentAccess hammers markDirty/checkDirty from separate
// goroutines. Run with -race; it fails if dirtyErr is not mutex-protected.
func TestEngineDirtyErrConcurrentAccess(t *testing.T) {
	engine := &Engine{}

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			engine.markDirty(errors.New("memory apply failed"))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = engine.checkDirty()
		}
	}()
	wg.Wait()

	require.Error(t, engine.checkDirty())
}
