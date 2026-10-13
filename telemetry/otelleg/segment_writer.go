package otelleg

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	segmentDirPerm  = 0o700
	segmentFilePerm = 0o600
	segmentPrefix   = "relurpify"
)

// segmentWriter writes OTLP-JSON batches to daily, size-bounded segment files
// under a directory and prunes segments older than the retention window.
//
// It is the local file leg's durability boundary. A shared, mutex-wrapped
// instance is handed to every signal's file transport, so concurrent exporters
// can never interleave partial lines (one Write call is one complete JSON
// object). Failure semantics are deliberately lenient: a write failure counts
// the drop, warns once, and disables the leg for the session rather than
// blocking or failing the runtime (D9).
type segmentWriter struct {
	dir       string
	prefix    string
	baseName  string
	retention time.Duration
	maxBytes  int64
	now       func() time.Time

	mu       sync.Mutex
	file     *os.File
	day      string
	size     int64
	disabled bool
	warned   atomic.Bool
	dropped  atomic.Uint64
}

// newSegmentWriter opens (or creates) the segment store. Directory creation and
// the initial segment open are best-effort: an unwritable directory disables the
// leg with a single warning rather than failing the chain. A non-empty
// baseName switches the writer to fixed-name segments (<baseName>.jsonl, no
// daily roll); empty uses the dated relurpify-YYYY-MM-DD convention.
func newSegmentWriter(dir, prefix, baseName string, retention time.Duration, maxSizeMB int, now func() time.Time) *segmentWriter {
	if strings.TrimSpace(prefix) == "" {
		prefix = segmentPrefix
	}
	if baseName != "" {
		baseName = strings.TrimSuffix(baseName, ".jsonl")
	}
	if now == nil {
		now = time.Now
	}
	w := &segmentWriter{
		dir:       dir,
		prefix:    prefix,
		baseName:  baseName,
		retention: retention,
		maxBytes:  int64(maxSizeMB) * 1024 * 1024,
		now:       now,
	}
	if err := os.MkdirAll(filepath.Clean(dir), segmentDirPerm); err != nil {
		w.disable("create segment directory: " + err.Error())
		return w
	}
	w.prune()
	w.roll(time.Time{}, 0)
	return w
}

// Write appends one complete OTLP-JSON object (the caller includes the trailing
// newline). It never returns an error: a failing disk must not make the runtime
// retry or block, so the failure is counted and the leg disabled instead.
func (w *segmentWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.disabled {
		w.dropped.Add(1)
		return len(p), nil
	}
	w.roll(w.now(), len(p))
	if w.file == nil {
		w.dropped.Add(1)
		return len(p), nil
	}
	n, err := w.file.Write(p)
	if err != nil {
		w.dropped.Add(1)
		w.disable("write segment: " + err.Error())
		return len(p), nil
	}
	w.size += int64(n)
	return n, nil
}

// Close releases the open segment. It is idempotent.
func (w *segmentWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// DroppedTotal returns the number of batches dropped by the file leg.
func (w *segmentWriter) DroppedTotal() uint64 {
	return w.dropped.Load()
}

// roll opens a fresh segment when the local day changed or the current segment
// would exceed the size bound. It must be called with w.mu held.
func (w *segmentWriter) roll(now time.Time, incoming int) {
	if w.disabled {
		return
	}
	if w.baseName != "" {
		w.rollFixed(incoming)
		return
	}
	if now.IsZero() {
		now = w.now()
	}
	day := now.Format("2006-01-02")
	if w.file != nil && w.day == day && (w.maxBytes <= 0 || w.size+int64(incoming) <= w.maxBytes) {
		return
	}
	rotate := w.file != nil
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			log.Printf("telemetry: closing segment failed: %v", err)
		}
		w.file = nil
	}

	name := fmt.Sprintf("%s-%s.jsonl", w.prefix, day)
	if rotate && w.day == day {
		// Same-day size overflow: suffix with the local wall clock so the
		// overflow segment is distinct and sortable.
		name = w.uniqueName(fmt.Sprintf("%s-%sT%s", w.prefix, day, now.Format("1504")))
	}
	if !w.openSegment(name) {
		return
	}
	w.day = day
	if rotate {
		w.prune()
	}
}

// rollFixed opens the fixed-name segment, rotating only when the size bound
// would be exceeded (overflow segments get a distinct sortable suffix). It must
// be called with w.mu held.
func (w *segmentWriter) rollFixed(incoming int) {
	if w.file != nil && (w.maxBytes <= 0 || w.size+int64(incoming) <= w.maxBytes) {
		return
	}
	rotate := w.file != nil
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			log.Printf("telemetry: closing segment failed: %v", err)
		}
		w.file = nil
	}
	name := w.baseName + ".jsonl"
	if rotate {
		// Size overflow: suffix with the local wall clock so the overflow
		// segment is distinct and sortable.
		name = w.uniqueName(fmt.Sprintf("%sT%s", w.baseName, w.now().Format("150405")))
	}
	if !w.openSegment(name) {
		return
	}
	if rotate {
		w.prune()
	}
}

// openSegment opens dir/name for append and records it as the current segment.
// It must be called with w.mu held; a failure disables the writer.
func (w *segmentWriter) openSegment(name string) bool {
	path := filepath.Join(filepath.Clean(w.dir), name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, segmentFilePerm) //nolint:gosec // path is under the operator-configured telemetry directory
	if err != nil {
		w.disable("open segment: " + err.Error())
		return false
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		w.disable("stat segment: " + err.Error())
		return false
	}
	w.file = f
	w.size = info.Size()
	return true
}

// uniqueName returns base+".jsonl" if free, otherwise base-N.jsonl for the first
// free N. It is used only for size overflow.
func (w *segmentWriter) uniqueName(base string) string {
	name := base + ".jsonl"
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(filepath.Clean(w.dir), name)); os.IsNotExist(err) {
			return name
		}
		name = fmt.Sprintf("%s-%d.jsonl", base, i)
	}
}

// prune deletes segments whose modification time is older than the retention
// window. It is called at open and after each rotation. A failure to delete one
// segment is logged and skipped; pruning never aborts telemetry.
func (w *segmentWriter) prune() {
	if w.retention <= 0 {
		return
	}
	cutoff := w.now().Add(-w.retention)
	entries, err := os.ReadDir(filepath.Clean(w.dir))
	if err != nil {
		log.Printf("telemetry: segment prune list failed: %v", err)
		return
	}
	// Fixed-name mode prunes only the size-overflow siblings; the live base
	// segment is never a prune candidate.
	liveName := ""
	stem := w.prefix + "-"
	if w.baseName != "" {
		liveName = w.baseName + ".jsonl"
		stem = w.baseName
	}
	isSegment := func(name string) bool {
		if !strings.HasSuffix(name, ".jsonl") {
			return false
		}
		if !strings.HasPrefix(name, stem) {
			return false
		}
		return w.baseName == "" || name != liveName
	}
	for _, entry := range entries {
		if entry.IsDir() || !isSegment(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(filepath.Clean(w.dir), entry.Name())); err != nil {
			log.Printf("telemetry: segment prune failed for %s: %v", entry.Name(), err)
		}
	}
}

// disable turns the leg off for the session and logs the degradation exactly
// once. Callers must hold w.mu (or be the constructor).
func (w *segmentWriter) disable(reason string) {
	w.disabled = true
	if w.warned.CompareAndSwap(false, true) {
		log.Printf("telemetry: file leg degraded, dropping batches: %s", reason)
	}
}
