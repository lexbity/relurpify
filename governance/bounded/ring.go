package bounded

import "sync"

// Ring is a fixed-capacity ring buffer: appending past capacity overwrites
// the oldest element. Snapshot returns the retained elements oldest-first.
// All methods are safe for concurrent use. The zero value is not usable —
// construct with NewRing.
type Ring[T any] struct {
	mu       sync.Mutex
	elements []T
	cap      int
	next     int // index of the oldest element
	size     int
}

// NewRing builds a ring holding at most cap elements. cap must be positive.
func NewRing[T any](cap int) *Ring[T] {
	if cap <= 0 {
		panic("bounded: ring capacity must be positive")
	}
	return &Ring[T]{elements: make([]T, cap), cap: cap}
}

// Append adds v, overwriting the oldest element once the ring is full.
func (r *Ring[T]) Append(v T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	overwrite := r.size == r.cap
	r.elements[(r.next+r.size)%r.cap] = v
	if overwrite {
		r.next = (r.next + 1) % r.cap
	} else {
		r.size++
	}
}

// Snapshot copies the retained elements into a new slice, oldest first.
func (r *Ring[T]) Snapshot() []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]T, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.elements[(r.next+i)%r.cap]
	}
	return out
}

// Len reports the number of retained elements.
func (r *Ring[T]) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.size
}
