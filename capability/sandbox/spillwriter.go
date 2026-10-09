package sandbox

import (
	"bytes"
	"io"
	"sync"
	"sync/atomic"
)

// spillWriter records all bytes written and signals when the stream exceeds
// the configured ceiling so the runner can tear down the container. The
// in-memory preview holds everything up to the ceiling (in practice limited by
// the ceiling teardown) and is what gets spilled to disk when the ceiling is
// hit (SBH-1 D-12).
type spillWriter struct {
	preview  bytes.Buffer
	total    int64
	ceiling  int64
	exceeded atomic.Bool
	excess   chan struct{}
	once     sync.Once
}

func newSpillWriter(ceiling int64) *spillWriter {
	if ceiling <= 0 {
		ceiling = 32 * 1024 * 1024
	}
	return &spillWriter{ceiling: ceiling, excess: make(chan struct{}, 1)}
}

func (w *spillWriter) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	n := len(p)
	// Retain exactly the prefix that fits under the ceiling; the crossing
	// chunk's overflow is dropped (D-12: the retained prefix is the spill).
	if room := w.ceiling - w.total; room > 0 {
		keep := int64(n)
		if int64(n) > room {
			keep = room
		}
		_, _ = w.preview.Write(p[:keep])
	}
	w.total += int64(n)
	if w.total > w.ceiling {
		w.markExceeded()
	}
	return n, nil
}

// markExceeded fires the exceed signal exactly once (buffered 1, non-blocking).
func (w *spillWriter) markExceeded() {
	w.once.Do(func() {
		w.exceeded.Store(true)
		w.excess <- struct{}{}
	})
}

// Exceeded reports whether the stream crossed the ceiling.
func (w *spillWriter) Exceeded() bool {
	return w != nil && w.exceeded.Load()
}

// WaitExceeded returns the once-fired exceed signal; a watchdog can select on
// it to tear the process down. A nil writer returns a never-firing channel.
func (w *spillWriter) WaitExceeded() <-chan struct{} {
	if w == nil {
		return nil
	}
	return w.excess
}

func (w *spillWriter) String() string {
	if w == nil {
		return ""
	}
	return w.preview.String()
}

func (w *spillWriter) Len() int {
	if w == nil {
		return 0
	}
	return int(w.total)
}

var _ io.Writer = (*spillWriter)(nil)
var _ io.Writer = (*bytes.Buffer)(nil)
