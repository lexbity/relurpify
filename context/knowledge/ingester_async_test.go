package knowledge

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// lockedLogBuffer is a concurrency-safe writer for capturing the global logger
// output while a background ingestion goroutine is still running.
type lockedLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestIngestToolResultAsyncLogsFailure(t *testing.T) {
	buf := &lockedLogBuffer{}
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	// A nil store makes IngestToolResult fail deterministically.
	IngestToolResultAsync(context.Background(), &OutputIngester{}, "shell", []byte("output"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "async tool result ingestion failed") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected async ingestion failure log, got %q", buf.String())
}

func TestIngestObservationAsyncLogsFailure(t *testing.T) {
	buf := &lockedLogBuffer{}
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	IngestObservationAsync(context.Background(), &OutputIngester{}, "observation")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "async observation ingestion failed") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected async ingestion failure log, got %q", buf.String())
}
