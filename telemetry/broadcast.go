package telemetry

import (
	"sync"
	"sync/atomic"
)

// EventToolEdited is emitted when a write-class capability invocation succeeds.
const EventToolEdited EventType = "tool.edited"

type subscriber struct {
	ch      chan Event
	dropped atomic.Uint64
}

// BroadcastSink is an in-memory, subscribable Telemetry implementation.
// Emit is non-blocking and goroutine-safe; slow consumers have events dropped
// rather than stalling the producer. Each subscriber receives a buffered channel;
// when full, the newest event is dropped and a per-subscriber counter is incremented.
type BroadcastSink struct {
	mu     sync.RWMutex
	subs   map[uint64]*subscriber
	nextID uint64
	seq    atomic.Uint64
	closed bool
}

// NewBroadcastSink returns a ready-to-use BroadcastSink.
func NewBroadcastSink() *BroadcastSink {
	return &BroadcastSink{
		subs: make(map[uint64]*subscriber),
	}
}

// Emit delivers the event to all subscribers via non-blocking sends.
// If a subscriber's buffer is full the event is dropped and its drop counter
// is incremented. A monotonic Seq is stamped when the incoming Seq is 0.
// Goroutine-safe: acquires RLock.
func (b *BroadcastSink) Emit(ev Event) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	if ev.Seq == 0 {
		ev.Seq = b.seq.Add(1)
	}
	for _, sub := range b.subs {
		select {
		case sub.ch <- ev:
		default:
			sub.dropped.Add(1)
		}
	}
	b.mu.RUnlock()
}

// Subscribe registers a new subscriber with a buffered channel of the given
// capacity (defaults to 256 when buffer <= 0). Returns the receive channel
// and a cancel function that removes the subscriber and closes its channel.
// After Close, Subscribe returns an already-closed channel and a no-op cancel.
// Goroutine-safe: acquires Lock.
func (b *BroadcastSink) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 256
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan Event, buffer)
		close(ch)
		return ch, func() {}
	}

	ch := make(chan Event, buffer)
	id := b.nextID
	b.nextID++
	b.subs[id] = &subscriber{ch: ch}

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if sub, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(sub.ch)
		}
	}
	return ch, cancel
}

// Close terminates all subscribers by closing their channels and clearing
// the subscriber map. Idempotent and goroutine-safe: acquires Lock.
func (b *BroadcastSink) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, sub := range b.subs {
		close(sub.ch)
	}
	b.subs = make(map[uint64]*subscriber)
}

// SubscriberCount returns the number of active subscribers.
// Goroutine-safe: acquires RLock.
func (b *BroadcastSink) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subs)
}

// DroppedTotal returns the sum of dropped events across all current subscribers.
// Goroutine-safe: acquires RLock.
func (b *BroadcastSink) DroppedTotal() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var total uint64
	for _, sub := range b.subs {
		total += sub.dropped.Load()
	}
	return total
}
