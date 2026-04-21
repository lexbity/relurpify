package gateway

import (
	"context"
	"sync"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/event"
)

type testEventLog struct {
	mu        sync.RWMutex
	events    map[string][]core.FrameworkEvent
	snapshots map[string]testSnapshot
}

type testSnapshot struct {
	seq  uint64
	data []byte
}

func newTestEventLog() *testEventLog {
	return &testEventLog{
		events:    map[string][]core.FrameworkEvent{},
		snapshots: map[string]testSnapshot{},
	}
}

func (l *testEventLog) Append(_ context.Context, partition string, events []core.FrameworkEvent) ([]uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	current := append([]core.FrameworkEvent(nil), l.events[partition]...)
	seqs := make([]uint64, 0, len(events))
	for _, evt := range events {
		evt.Partition = partition
		evt.Seq = uint64(len(current) + 1)
		current = append(current, evt)
		seqs = append(seqs, evt.Seq)
	}
	l.events[partition] = current
	return seqs, nil
}

func (l *testEventLog) Read(_ context.Context, partition string, afterSeq uint64, limit int, _ bool) ([]core.FrameworkEvent, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	source := l.events[partition]
	out := make([]core.FrameworkEvent, 0, len(source))
	for _, evt := range source {
		if evt.Seq <= afterSeq {
			continue
		}
		out = append(out, evt)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (l *testEventLog) ReadByType(_ context.Context, partition string, typePrefix string, afterSeq uint64, limit int) ([]core.FrameworkEvent, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	source := l.events[partition]
	out := make([]core.FrameworkEvent, 0, len(source))
	for _, evt := range source {
		if evt.Seq <= afterSeq {
			continue
		}
		if typePrefix != "" && len(evt.Type) >= len(typePrefix) && evt.Type[:len(typePrefix)] != typePrefix {
			continue
		}
		if typePrefix != "" && len(evt.Type) < len(typePrefix) {
			continue
		}
		out = append(out, evt)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (l *testEventLog) LastSeq(_ context.Context, partition string) (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return uint64(len(l.events[partition])), nil
}

func (l *testEventLog) TakeSnapshot(_ context.Context, partition string, seq uint64, data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.snapshots[partition] = testSnapshot{seq: seq, data: append([]byte(nil), data...)}
	return nil
}

func (l *testEventLog) LoadSnapshot(_ context.Context, partition string) (uint64, []byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	snapshot := l.snapshots[partition]
	return snapshot.seq, append([]byte(nil), snapshot.data...), nil
}

func (l *testEventLog) Close() error {
	return nil
}

var _ event.Log = (*testEventLog)(nil)
