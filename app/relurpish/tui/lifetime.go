package tui

import (
	"context"
	"sync"
)

// hostLifetime is shared by Bubble Tea's value copies. It owns cancellation and
// every registered resource, including inactive cached surfaces. A subscription
// arriving after shutdown is immediately released, never silently leaked.
type hostLifetime struct {
	mu        sync.Mutex
	once      sync.Once
	closed    bool
	cancel    context.CancelFunc
	callbacks []func()
}

func (l *hostLifetime) add(closeResource func()) {
	if closeResource == nil {
		return
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		closeResource()
		return
	}
	l.callbacks = append(l.callbacks, closeResource)
	l.mu.Unlock()
}

func (l *hostLifetime) close() {
	l.once.Do(func() {
		l.mu.Lock()
		l.closed = true
		callbacks := l.callbacks
		l.callbacks = nil
		l.mu.Unlock()
		l.cancel()
		// Resource callbacks execute outside the lock, in reverse ownership order.
		// Defers release earlier resources even if a later cleanup panics.
		for _, callback := range callbacks {
			defer callback()
		}
	})
}

func (l *hostLifetime) ownSurface(state *surfaceState) {
	if state.chat != nil {
		l.add(state.chat.Cleanup)
	}
	if state.region1 != nil {
		l.add(state.region1.Cleanup)
	}
}
