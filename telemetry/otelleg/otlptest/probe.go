// Package otlptest is a test-only probe for the OTLP-JSON file leg: it writes
// telemetry events through a real file-only otelleg.Chain and reads the
// resulting segments back. Cross-package tests (agenttest, dev-agent-cli, TUI,
// e2e) use it so every consumer exercises the same real exporter path instead
// of hand-rolled fixtures.
package otlptest

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/telemetry"
	"codeburg.org/lexbit/relurpify/telemetry/otelleg"
)

// SegmentName is the fixed segment name wirings under test use when artifact
// paths must be stable.
const SegmentName = "agenttest-otlp"

// WriteSegments emits events through a real file-only chain writing
// <dir>/<SegmentName>.jsonl and closes the chain (flushing every provider).
// The chain's fail-closed construction and drop counters are the real ones —
// a construction failure fails the test.
func WriteSegments(t *testing.T, dir string, events []telemetry.Event) {
	t.Helper()
	chain, err := otelleg.New(context.Background(), otelleg.Settings{
		Enabled:          true,
		FileEnabled:      true,
		FileDir:          dir,
		FileNameOverride: SegmentName,
	})
	if err != nil {
		t.Fatalf("otelleg.New: %v", err)
	}
	for _, ev := range events {
		chain.Emit(ev)
	}
	if err := chain.Close(context.Background()); err != nil {
		t.Fatalf("close chain: %v", err)
	}
}

// LoadRecords reads every log record from the segments under dir, in file
// order. A missing or empty segment store fails the test.
func LoadRecords(t *testing.T, dir string) []otelleg.LogRecord {
	t.Helper()
	records, err := otelleg.ReadSegmentDirLogRecords(dir)
	if err != nil {
		t.Fatalf("read segments under %s: %v", dir, err)
	}
	return records
}

// RecordsNamed returns the records whose eventName equals name.
func RecordsNamed(records []otelleg.LogRecord, name string) []otelleg.LogRecord {
	var out []otelleg.LogRecord
	for _, rec := range records {
		if rec.Name == name {
			out = append(out, rec)
		}
	}
	return out
}
