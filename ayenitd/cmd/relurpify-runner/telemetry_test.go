package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"codeburg.org/lexbit/relurpify/telemetry"
)

func TestNewFileTelemetryWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runner.jsonl")
	tel, err := newFileTelemetry(path)
	if err != nil {
		t.Fatalf("newFileTelemetry: %v", err)
	}
	tel.Emit(telemetry.Event{Type: telemetry.EventRunnerStarted, Message: "up"})
	tel.Emit(telemetry.Event{Type: telemetry.EventRunnerStopped, Message: "down"})

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read telemetry log: %v", err)
	}
	lines := nonEmptyLines(string(data))
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	var ev telemetry.Event
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatalf("line 0 is not JSON: %v", err)
	}
	if ev.Type != telemetry.EventRunnerStarted {
		t.Errorf("line 0 type = %q", ev.Type)
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	return out
}
