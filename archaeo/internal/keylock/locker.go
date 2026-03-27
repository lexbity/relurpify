package keylock

import "sync"

type lockerEntry struct {
	mu   sync.Mutex
	refs int
}

type Locker struct {
	entries sync.Map
}

func (l *Locker) With(key string, fn func() error) error {
	if fn == nil {
		return nil
	}
	entry := l.acquire(key)
	entry.mu.Lock()
	defer func() {
		entry.mu.Unlock()
		l.release(key, entry)
	}()
	return fn()
}

func (l *Locker) acquire(key string) *lockerEntry {
	if key == "" {
		key = "<default>"
	}
	for {
		if current, ok := l.entries.Load(key); ok {
			entry := current.(*lockerEntry)
			entry.mu.Lock()
			if actual, ok := l.entries.Load(key); ok && actual == current {
				entry.refs++
				entry.mu.Unlock()
				return entry
			}
			entry.mu.Unlock()
			continue
		}
		entry := &lockerEntry{refs: 1}
		actual, loaded := l.entries.LoadOrStore(key, entry)
		if !loaded {
			return entry
		}
		existing := actual.(*lockerEntry)
		existing.mu.Lock()
		if actual2, ok := l.entries.Load(key); ok && actual2 == actual {
			existing.refs++
			existing.mu.Unlock()
			return existing
		}
		existing.mu.Unlock()
	}
}

func (l *Locker) release(key string, entry *lockerEntry) {
	if key == "" {
		key = "<default>"
	}
	if entry == nil {
		return
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.refs--
	if entry.refs <= 0 {
		l.entries.CompareAndDelete(key, entry)
	}
}
