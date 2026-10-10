// badger_log.go implements the framework event.Log contract on Badger v4
// (Decision 5). It is a durable causal/audit record scored alongside the
// JSONL operational telemetry; the two serve different questions, and a
// failure to open this store must never take the runtime down (NFR-4 — the
// composition root decides policy, this type only surfaces the error).
package event

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// secureDirMode matches capability/fs SecureDirMode; the event log creates
// only its own Badger directory and must not import platform packages
// (telemetry → telemetry/event stays dependency-clean).
const secureDirMode os.FileMode = 0o700

// BadgerLog is a Badger-backed append-only framework event log.
type BadgerLog struct {
	db *badger.DB
	mu sync.Mutex // serializes Append and the per-partition seq read-modify-write
}

// NewBadgerLog opens (or creates) a badger event log at path.
func NewBadgerLog(path string) (*BadgerLog, error) {
	if path == "" {
		return nil, fmt.Errorf("event: badger log path is required")
	}
	if err := os.MkdirAll(path, secureDirMode); err != nil {
		return nil, fmt.Errorf("event: create badger dir: %w", err)
	}
	opts := badger.DefaultOptions(path).
		WithLogger(nil).
		WithNumCompactors(2).
		WithMemTableSize(16 << 20)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("event: open badger log %q: %w", path, err)
	}
	return &BadgerLog{db: db}, nil
}

func eventKey(partition string, seq uint64) []byte {
	key := make([]byte, 0, len(partition)+len("p//e/")+20)
	key = append(key, 'p', '/')
	key = append(key, partition...)
	key = append(key, '/', 'e', '/', 0x00)
	return appendSeqKey(key, seq)
}

func seqKey(partition string) []byte {
	return []byte("p/" + partition + "/meta/seq")
}

func snapshotKey(partition string) []byte {
	return []byte("p/" + partition + "/meta/snapshot")
}

func appendSeqKey(dst []byte, seq uint64) []byte {
	var buf [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(buf[:], seq)
	return append(dst, buf[:n]...)
}

// seqFromKey extracts the seq encoded after the 0x00 delimiter that
// terminates the static part of an event key ("p/<partition>/e/\x00<varint>").
// The delimiter keeps lexicographic order across varint widths: without it a
// longer varint's first byte would compare before a shorter one's payload.
func seqFromKey(key []byte) (uint64, error) {
	idx := -1
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == 0x00 {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0, fmt.Errorf("event: bad key %q", string(key))
	}
	seq, read := binary.Uvarint(key[idx+1:])
	if read <= 0 {
		return 0, fmt.Errorf("event: bad seq in key %q", string(key))
	}
	return seq, nil
}

// Append stores the events with per-partition monotonic sequences assigned
// in this call's order, fulfilling the idempotent causal-record contract.
func (l *BadgerLog) Append(ctx context.Context, partition string, events []FrameworkEvent) ([]uint64, error) {
	if l == nil || l.db == nil {
		return nil, fmt.Errorf("event: log is closed")
	}
	if len(events) == 0 {
		return []uint64{}, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	var last uint64
	if err := l.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(seqKey(partition))
		if err == badger.ErrKeyNotFound {
			last = 0
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			if len(v) != 8 {
				return fmt.Errorf("event: corrupt seq record for partition %q", partition)
			}
			last = binary.BigEndian.Uint64(v)
			return nil
		})
	}); err != nil {
		return nil, err
	}

	seqs := make([]uint64, len(events))
	payloads := make([][]byte, len(events))
	for i := range events {
		last++
		events[i].Seq = last
		events[i].Partition = partition
		data, err := json.Marshal(events[i])
		if err != nil {
			return nil, fmt.Errorf("event: marshal framework event: %w", err)
		}
		seqs[i] = last
		payloads[i] = data
	}

	var seqBytes [8]byte
	binary.BigEndian.PutUint64(seqBytes[:], last)
	err := l.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(seqKey(partition), seqBytes[:]); err != nil {
			return err
		}
		for i := range events {
			if err := txn.Set(eventKey(partition, seqs[i]), payloads[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return seqs, nil
}

// Read returns events after afterSeq for the partition. With follow, it
// keeps polling until either new events (bounded by limit) appear, or ctx
// is cancelled.
func (l *BadgerLog) Read(ctx context.Context, partition string, afterSeq uint64, limit int, follow bool) ([]FrameworkEvent, error) {
	if l == nil || l.db == nil {
		return nil, fmt.Errorf("event: log is closed")
	}
	out, err := l.readOnce(partition, afterSeq, limit)
	if err != nil || !follow {
		return out, err
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if len(out) > 0 && (limit <= 0 || len(out) >= limit) {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-ticker.C:
			out, err = l.readOnce(partition, afterSeq, limit)
			if err != nil {
				return nil, err
			}
		}
	}
}

func (l *BadgerLog) readOnce(partition string, afterSeq uint64, limit int) ([]FrameworkEvent, error) {
	var out []FrameworkEvent
	opts := badger.DefaultIteratorOptions
	opts.PrefetchValues = true
	err := l.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte("p/" + partition + "/e/")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			if limit > 0 && len(out) >= limit {
				break
			}
			seq, err := seqFromKey(item.KeyCopy(nil))
			if err != nil || seq <= afterSeq {
				continue
			}
			if err := item.Value(func(v []byte) error {
				var ev FrameworkEvent
				if err := json.Unmarshal(v, &ev); err != nil {
					return fmt.Errorf("event: corrupt stored event: %w", err)
				}
				out = append(out, ev)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// ReadByType returns events after afterSeq whose Type carries typePrefix.
func (l *BadgerLog) ReadByType(ctx context.Context, partition string, typePrefix string, afterSeq uint64, limit int) ([]FrameworkEvent, error) {
	if l == nil || l.db == nil {
		return nil, fmt.Errorf("event: log is closed")
	}
	var out []FrameworkEvent
	opts := badger.DefaultIteratorOptions
	err := l.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(opts)
		defer it.Close()
		prefix := []byte("p/" + partition + "/e/")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			if limit > 0 && len(out) >= limit {
				break
			}
			seq, err := seqFromKey(item.KeyCopy(nil))
			if err != nil || seq <= afterSeq {
				continue
			}
			if err := item.Value(func(v []byte) error {
				var ev FrameworkEvent
				if err := json.Unmarshal(v, &ev); err != nil {
					return fmt.Errorf("event: corrupt stored event: %w", err)
				}
				if len(ev.Type) >= len(typePrefix) && ev.Type[:len(typePrefix)] == typePrefix {
					out = append(out, ev)
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// LastSeq returns the last assigned sequence for a partition.
func (l *BadgerLog) LastSeq(_ context.Context, partition string) (uint64, error) {
	if l == nil || l.db == nil {
		return 0, fmt.Errorf("event: log is closed")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var last uint64
	err := l.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(seqKey(partition))
		if err == badger.ErrKeyNotFound {
			last = 0
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			if len(v) != 8 {
				return fmt.Errorf("event: corrupt seq record for partition %q", partition)
			}
			last = binary.BigEndian.Uint64(v)
			return nil
		})
	})
	return last, err
}

// TakeSnapshot persists a materializer snapshot with its sequence.
func (l *BadgerLog) TakeSnapshot(_ context.Context, partition string, seq uint64, data []byte) error {
	if l == nil || l.db == nil {
		return fmt.Errorf("event: log is closed")
	}
	record, err := json.Marshal(struct {
		Seq  uint64 `json:"seq"`
		Data []byte `json:"data"`
	}{Seq: seq, Data: data})
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.db.Update(func(txn *badger.Txn) error {
		return txn.Set(snapshotKey(partition), record)
	})
}

// LoadSnapshot restores the stored snapshot with its sequence; zero
// sequence and nil data when none was taken.
func (l *BadgerLog) LoadSnapshot(_ context.Context, partition string) (uint64, []byte, error) {
	if l == nil || l.db == nil {
		return 0, nil, fmt.Errorf("event: log is closed")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var (
		seq  uint64
		data []byte
	)
	err := l.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(snapshotKey(partition))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			var record struct {
				Seq  uint64 `json:"seq"`
				Data []byte `json:"data"`
			}
			if err := json.Unmarshal(v, &record); err != nil {
				return err
			}
			seq = record.Seq
			data = record.Data
			return nil
		})
	})
	return seq, data, err
}

// Close flushes and releases the Badger directory. Safe against double close.
func (l *BadgerLog) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.db.Close()
	l.db = nil
	return err
}

// Compile-time interface check.
var _ Log = (*BadgerLog)(nil)
