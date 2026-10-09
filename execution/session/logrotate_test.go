package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func writeLogLines(t *testing.T, w *rotatingWriter, bytes int) {
	t.Helper()
	payload := []byte(strings.Repeat("x", 1024))
	written := 0
	for written < bytes {
		n, _ := w.Write(payload)
		if n == 0 {
			t.Fatal("write made no progress")
		}
		written += n
	}
}

func rotatedFiles(t *testing.T, dir, base string) []string {
	t.Helper()
	var found []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), base+".") {
			found = append(found, e.Name())
		}
	}
	return found
}

// TestRotatingWriterRotatesAtCap: writing past 10 MiB produces exactly the
// kept rotations plus the current file, oldest content verifiable via .1
// (AC-6).
func TestRotatingWriterRotatesAtCap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.log")
	w, err := NewRotatingWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	var rotations []int
	w.onRotate = func(index int) { rotations = append(rotations, index) }

	// 10.5 MiB in 1 KiB chunks forces one rotation.
	writeLogLines(t, w, 10<<20+512*1024)

	files := rotatedFiles(t, dir, "workspace.log")
	if len(files) != 1 || files[0] != "workspace.log.1" {
		t.Fatalf("rotated files = %v, want [workspace.log.1]", files)
	}
	if len(rotations) == 0 {
		t.Fatal("rotation callback never fired")
	}
	// Current file restarted from zero and holds the tail of the stream.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 || info.Size() > w.maxBytes {
		t.Fatalf("current log size %d outside (0, %d]", info.Size(), w.maxBytes)
	}
}

// TestRotatingWriterKeepsBound: sustained writes keep at most `keep` rotated
// files; oldest rotations are deleted, never accumulated.
func TestRotatingWriterKeepsBound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.log")
	w, err := NewRotatingWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	// ~3.5 rotations worth.
	writeLogLines(t, w, int(w.maxBytes)*3+int(w.maxBytes)/2)

	files := rotatedFiles(t, dir, "workspace.log")
	if len(files) > w.keep {
		t.Fatalf("rotated files = %d, want <= %d", len(files), w.keep)
	}
}

// TestRotatingWriterPrunesAgedRotations: rotated files older than the
// retention window are deleted on open (telemetry-retention discipline).
func TestRotatingWriterPrunesAgedRotations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.log")

	aged := rotatedPath(path, 1)
	if err := os.WriteFile(aged, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-logRotateRetention - time.Hour)
	if err := os.Chtimes(aged, past, past); err != nil {
		t.Fatal(err)
	}
	fresh := rotatedPath(path, 2)
	if err := os.WriteFile(fresh, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	w, err := NewRotatingWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w.Close() }()

	if _, err := os.Stat(aged); !os.IsNotExist(err) {
		t.Fatal("aged rotation not pruned")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh rotation was pruned")
	}
}

// TestRotatingWriterCrashBetweenRenames: interrupting a rotation (closing
// the writer mid-rotate and reopening) loses no already-flushed line — the
// rename chain is forward-order and atomic per rename (AC-6).
func TestRotatingWriterCrashBetweenRenames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.log")

	w, err := NewRotatingWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	line1 := "line-before-rotation\n"
	if _, err := w.Write([]byte(line1)); err != nil {
		t.Fatal(err)
	}
	// Simulate the crash: force rotation to move current→.1, then abandon
	// the writer without completing the create step.
	if err := w.rotateForTest(); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()

	// Reopen: the flushed line survives in the rotated file.
	data, err := os.ReadFile(rotatedPath(path, 1))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), strings.TrimSpace(line1)) {
		t.Fatalf("flushed line lost across rotation crash: %q", string(data))
	}

	// A new writer continues on a fresh current file.
	w2, err := NewRotatingWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w2.Close() }()
	if _, err := w2.Write([]byte("resumed\n")); err != nil {
		t.Fatal(err)
	}
}

// TestRotatingWriterConcurrentWriters: two handles on one workspace produce
// no corruption — lines may interleave (documented single-writer-per-process
// assumption) but every written line remains intact (R8).
func TestRotatingWriterConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "workspace.log")
	w1, err := NewRotatingWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	w2, err := NewRotatingWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = w1.Close() }()
	defer func() { _ = w2.Close() }()

	var wg sync.WaitGroup
	lineOf := func(tag string, i int) []byte {
		return []byte(fmt.Sprintf("writer-%s-line-%04d-padding-padding-padding\n", tag, i))
	}
	for i := 0; i < 500; i++ {
		wg.Add(2)
		go func(i int) { defer wg.Done(); _, _ = w1.Write(lineOf("a", i)) }(i)
		go func(i int) { defer wg.Done(); _, _ = w2.Write(lineOf("b", i)) }(i)
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Every complete line must be exactly one writer's line — no torn writes.
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		okA := strings.HasPrefix(line, "writer-a-line-") && strings.HasSuffix(line, "padding")
		okB := strings.HasPrefix(line, "writer-b-line-") && strings.HasSuffix(line, "padding")
		if !okA && !okB {
			t.Fatalf("corrupted line: %q", line)
		}
	}
}

// TestRotatingWriterCloseIdempotent: double-Close is once-guarded with a
// sticky result.
func TestRotatingWriterCloseIdempotent(t *testing.T) {
	w, err := NewRotatingWriter(filepath.Join(t.TempDir(), "workspace.log"))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
