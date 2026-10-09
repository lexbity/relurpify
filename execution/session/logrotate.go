package session

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Workspace log rotation bounds (§ operational hygiene): the current log
// rotates at 10 MiB; 5 rotated files are kept; rotated files older than 30
// days are pruned. Default steady-state cap ≈ 60 MiB per workspace.
const (
	logRotateMaxBytes  = 10 << 20 // 10 MiB
	logRotateKeepFiles = 5
	logRotateRetention = 30 * 24 * time.Hour
)

// rotatingWriter is an io.WriteCloser that rotates its log file by
// rename-then-create once the current file exceeds maxBytes. Rotation keeps
// logRotateKeepFiles rotated files (path.1 = newest … path.5 = oldest) and is
// crash-safe: renames run in forward order on the same filesystem, so an
// interrupted rotation loses at most the oldest rotated file — the one the
// next rotation would delete anyway. Already-flushed lines are never lost
// (rename is atomic). Write errors drop the line and keep serving; they must
// never take the process down.
type rotatingWriter struct {
	mu        sync.Mutex
	path      string
	file      *os.File
	size      int64
	maxBytes  int64
	keep      int
	retention time.Duration
	clock     func() time.Time
	// onRotate and onWriteFailed observe rotation and write-failure
	// transitions (telemetry hooks; counters are the contract).
	onRotate      func(index int)
	onWriteFailed func(err error)
	closeOnce     sync.Once
	closeErr      error
}

// NewRotatingWriter opens (or creates) the log at path with rotation
// enabled and prunes rotated files older than the retention window.
func NewRotatingWriter(path string) (*rotatingWriter, error) {
	f, err := os.OpenFile(filepath.Clean(path), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat log: %w", err)
	}
	w := &rotatingWriter{
		path:      path,
		file:      f,
		size:      info.Size(),
		maxBytes:  logRotateMaxBytes,
		keep:      logRotateKeepFiles,
		retention: logRotateRetention,
		clock:     time.Now,
	}
	w.pruneRotated()
	return w, nil
}

// Write appends p, rotating first when the write would cross the size cap.
func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if int64(len(p))+w.size > w.maxBytes {
		if err := w.rotateLocked(); err != nil {
			w.reportWriteFailed(err)
			return 0, nil // drop the line; keep serving
		}
	}
	if w.file == nil {
		return 0, nil
	}
	n, err := w.file.Write(p)
	if err != nil {
		w.reportWriteFailed(err)
		return n, nil // drop the line; keep serving
	}
	w.size += int64(n)
	return n, nil
}

// Close flushes and closes the current file. Safe to call more than once;
// the first result is sticky.
func (w *rotatingWriter) Close() error {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.file != nil {
			_ = w.file.Sync()
			w.closeErr = w.file.Close()
			w.file = nil
		}
	})
	return w.closeErr
}

// rotateLocked rolls path.4→path.5 … path.1→path.2, path→path.1, then
// creates a fresh current file. Forward-order renames never clobber a file
// that still holds the only copy of a line. Caller holds w.mu.
func (w *rotatingWriter) rotateLocked() error {
	if w.file != nil {
		_ = w.file.Sync()
		_ = w.file.Close()
		w.file = nil
	}
	for i := w.keep - 1; i >= 1; i-- {
		from := rotatedPath(w.path, i)
		to := rotatedPath(w.path, i+1)
		if _, err := os.Stat(from); err != nil {
			continue
		}
		_ = os.Remove(to)
		if err := os.Rename(from, to); err != nil {
			return fmt.Errorf("rotate %s: %w", from, err)
		}
		if w.onRotate != nil {
			w.onRotate(i + 1)
		}
	}
	if err := os.Rename(w.path, rotatedPath(w.path, 1)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rotate current: %w", err)
	}
	if w.onRotate != nil {
		w.onRotate(1)
	}
	f, err := os.OpenFile(filepath.Clean(w.path), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create log: %w", err)
	}
	w.file = f
	w.size = 0
	return nil
}

// pruneRotated deletes rotated files older than the retention window
// (reusing the telemetry-retention age-based pruning discipline).
func (w *rotatingWriter) pruneRotated() {
	now := w.clock()
	for i := 1; i <= w.keep; i++ {
		path := rotatedPath(w.path, i)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) > w.retention {
			_ = os.Remove(path)
		}
	}
}

func rotatedPath(path string, index int) string {
	return fmt.Sprintf("%s.%d", path, index)
}

func (w *rotatingWriter) reportWriteFailed(err error) {
	if w.onWriteFailed != nil {
		w.onWriteFailed(err)
		return
	}
	log.Printf("log.write_failed: path=%s err=%v; log line dropped", w.path, err)
}

// rotateForTest runs one rotation step synchronously (crash-injection seam
// for tests).
func (w *rotatingWriter) rotateForTest() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rotateLocked()
}
