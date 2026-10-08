package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeJSONLLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	var content []byte
	for _, line := range lines {
		if line == "" {
			continue
		}
		content = append(content, []byte(line)...)
		content = append(content, '\n')
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func eventLine(t *testing.T, et EventType, ts time.Time) string {
	t.Helper()
	data, err := json.Marshal(Event{Type: et, Timestamp: ts, Message: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestPruneJSONLDropsExpiredEvents(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "workspace.jsonl")
	old := eventLine(t, EventGraphStart, now.Add(-30*24*time.Hour))
	fresh := eventLine(t, EventGraphFinish, now.Add(-time.Hour))
	writeJSONLLines(t, path, old, fresh)

	if err := PruneJSONL(path, 7*24*time.Hour, now); err != nil {
		t.Fatalf("PruneJSONL: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if containsLine(got, "graph_start") {
		t.Fatalf("expired event survived pruning:\n%s", got)
	}
	if !containsLine(got, "graph_finish") {
		t.Fatalf("fresh event dropped by pruning:\n%s", got)
	}
}

func TestPruneJSONLKeepsMalformedAndBlankLines(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "workspace.jsonl")
	old := eventLine(t, EventGraphStart, now.Add(-30*24*time.Hour))
	writeJSONLLines(t, path, old, "not-json-at-all", "")

	if err := PruneJSONL(path, 7*24*time.Hour, now); err != nil {
		t.Fatalf("PruneJSONL: %v", err)
	}
	data, _ := os.ReadFile(path)
	got := string(data)
	if !containsLine(got, "not-json-at-all") {
		t.Fatal("unparseable line must be retained, not dropped")
	}
	if containsLine(got, "graph_start") {
		t.Fatal("expired event must still be pruned alongside malformed lines")
	}
}

func TestPruneJSONLNoOpOnMissingOrEmptyWindow(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "missing.jsonl")
	if err := PruneJSONL(path, 7*24*time.Hour, now); err != nil {
		t.Fatalf("missing file must be a no-op, got %v", err)
	}
	if err := PruneJSONL(path, 0, now); err != nil {
		t.Fatalf("zero retention must be a no-op, got %v", err)
	}
}

func TestPruneJSONLSkipsRewriteWhenNothingExpired(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "workspace.jsonl")
	fresh := eventLine(t, EventGraphFinish, now.Add(-time.Hour))
	writeJSONLLines(t, path, fresh)

	if err := PruneJSONL(path, 7*24*time.Hour, now); err != nil {
		t.Fatalf("PruneJSONL: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !containsLine(string(data), "graph_finish") {
		t.Fatal("fresh event must be retained unmodified")
	}
}

func containsLine(haystack, needle string) bool {
	for _, line := range splitNonEmpty(haystack) {
		if contains(line, needle) {
			return true
		}
	}
	return false
}

func splitNonEmpty(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
