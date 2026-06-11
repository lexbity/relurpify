package sandbox

import (
	"bytes"
	"io"
	"sync/atomic"
)

// spillWriter replaces the old capped-buffer pattern. It records all bytes
// written and signals when the stream exceeds the configured ceiling so the
// runner can tear down the container. The in-memory preview holds everything
// up to the ceiling (in practice limited by the ceiling teardown).
type spillWriter struct {
	preview  bytes.Buffer
	total    int64
	ceiling  int64
	exceeded atomic.Bool
}

func newSpillWriter(ceiling int64) *spillWriter {
	if ceiling <= 0 {
		ceiling = 32 * 1024 * 1024
	}
	return &spillWriter{ceiling: ceiling}
}

func (w *spillWriter) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	n := len(p)
	newTotal := w.total + int64(n)
	if newTotal > w.ceiling {
		w.exceeded.Store(true)
		w.total = newTotal
		return n, nil
	}
	w.total = newTotal
	_, _ = w.preview.Write(p)
	return n, nil
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
