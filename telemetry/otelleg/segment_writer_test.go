package otelleg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestSegmentWriterDailyRollover(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 13, 23, 59, 0, 0, time.UTC)
	w := newSegmentWriter(dir, segmentPrefix, "", 168*time.Hour, 256, func() time.Time { return now })
	defer func() { _ = w.Close() }()

	writeLine(t, w, `{"a":1}`+"\n")
	now = now.Add(2 * time.Minute) // crosses local midnight
	writeLine(t, w, `{"a":2}`+"\n")

	segments := segmentFiles(t, dir)
	if len(segments) != 2 {
		t.Fatalf("segment count = %d, want 2 (%v)", len(segments), segments)
	}
	for _, name := range segments {
		if lines := readLines(t, filepath.Join(dir, name)); len(lines) != 1 {
			t.Fatalf("segment %s has %d lines, want 1", name, len(lines))
		}
	}
}

func TestSegmentWriterSizeOverflow(t *testing.T) {
	dir := t.TempDir()
	w := newSegmentWriter(dir, segmentPrefix, "", 168*time.Hour, 1, nil)
	defer func() { _ = w.Close() }()

	// A payload larger than the 1 MiB bound forces an overflow segment.
	big := make([]byte, (1<<20)+64)
	for i := range big {
		big[i] = 'x'
	}
	writeLine(t, w, string(big)+"\n")
	writeLine(t, w, `{"small":true}`+"\n")

	if got := len(segmentFiles(t, dir)); got < 2 {
		t.Fatalf("expected an overflow segment, got %d segment(s)", got)
	}
}

func TestSegmentWriterPrune(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, segmentPrefix+"-2026-01-01.jsonl")
	fresh := filepath.Join(dir, segmentPrefix+"-2026-10-13.jsonl")
	foreign := filepath.Join(dir, "unrelated.jsonl")
	for _, p := range []string{old, fresh, foreign} {
		if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(foreign, past, past); err != nil {
		t.Fatal(err)
	}

	w := newSegmentWriter(dir, segmentPrefix, "", 168*time.Hour, 256, nil)
	defer func() { _ = w.Close() }()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old segment was not pruned: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh segment was pruned: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("unrelated file was pruned: %v", err)
	}
}

func TestSegmentWriterPruneSurvivesDirectoryEntry(t *testing.T) {
	dir := t.TempDir()
	// A directory whose name matches the segment pattern must not abort pruning.
	if err := os.Mkdir(filepath.Join(dir, segmentPrefix+"-2026-01-01.jsonl"), 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, segmentPrefix+"-2026-01-02.jsonl")
	if err := os.WriteFile(old, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	w := newSegmentWriter(dir, segmentPrefix, "", 168*time.Hour, 256, nil)
	defer func() { _ = w.Close() }()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old segment was not pruned after a directory entry: %v", err)
	}
}

func TestSegmentWriterConcurrentWritesKeepLinesWhole(t *testing.T) {
	dir := t.TempDir()
	w := newSegmentWriter(dir, segmentPrefix, "", 168*time.Hour, 256, nil)
	defer func() { _ = w.Close() }()

	const writers = 10
	const perWriter = 100
	done := make(chan struct{})
	for i := 0; i < writers; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			for j := 0; j < perWriter; j++ {
				writeLine(t, w, `{"writer":`+strconv.Itoa(id)+`,"seq":`+strconv.Itoa(j)+`}`+"\n")
			}
		}(i)
	}
	for i := 0; i < writers; i++ {
		<-done
	}

	total := 0
	for _, name := range segmentFiles(t, dir) {
		for _, line := range readLines(t, filepath.Join(dir, name)) {
			if !jsonValid([]byte(line)) {
				t.Fatalf("interleaved or partial line in %s: %q", name, line)
			}
			total++
		}
	}
	if total != writers*perWriter {
		t.Fatalf("wrote %d lines, want %d", total, writers*perWriter)
	}
}

func writeLine(t *testing.T, w *segmentWriter, line string) {
	t.Helper()
	if _, err := w.Write([]byte(line)); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func segmentFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, e.Name())
		}
	}
	return out
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // test helper reads a path under t.TempDir()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var lines []string
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

func jsonValid(b []byte) bool {
	var v any
	return json.Unmarshal(b, &v) == nil
}

func TestSegmentWriterFixedName(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 13, 23, 59, 0, 0, time.UTC)
	clock := now
	w := newSegmentWriter(dir, segmentPrefix, "agenttest-otlp", 168*time.Hour, 1, func() time.Time { return clock })
	defer func() { _ = w.Close() }()

	writeLine(t, w, `{"a":1}`+"\n")
	clock = clock.Add(2 * time.Minute) // crosses local midnight: fixed names never roll on the clock
	writeLine(t, w, `{"a":2}`+"\n")

	base := filepath.Join(dir, "agenttest-otlp.jsonl")
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("fixed-name segment missing: %v", err)
	}
	if lines := readLines(t, base); len(lines) != 2 {
		t.Fatalf("fixed-name segment has %d lines, want 2 (midnight must not roll a fixed name)", len(lines))
	}
}

func TestSegmentWriterFixedNamePrunesOverflowOnly(t *testing.T) {
	dir := t.TempDir()
	oldOverflow := filepath.Join(dir, "agenttest-otlpT010101.jsonl")
	live := filepath.Join(dir, "agenttest-otlp.jsonl")
	foreign := filepath.Join(dir, "relurpify-2026-01-01.jsonl")
	for path, age := range map[string]time.Duration{
		oldOverflow: 1000 * 24 * time.Hour,
		live:        1000 * 24 * time.Hour,
		foreign:     1000 * 24 * time.Hour,
	} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		past := time.Now().Add(-age)
		if err := os.Chtimes(path, past, past); err != nil {
			t.Fatal(err)
		}
	}
	w := newSegmentWriter(dir, segmentPrefix, "agenttest-otlp", 168*time.Hour, 256, nil)
	defer func() { _ = w.Close() }()

	if _, err := os.Stat(oldOverflow); !os.IsNotExist(err) {
		t.Fatalf("stale overflow segment not pruned: %v", err)
	}
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("live fixed-name segment must never be pruned: %v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("segments of other wirings must not be pruned: %v", err)
	}
}
