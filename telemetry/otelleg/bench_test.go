package otelleg

// Emit-path benchmarks (NFR-8, AC-5). The gate: the export leg's Emit p99
// must stay ≤ 25 µs/op at scale — telemetry conversion is O(metadata) and
// nothing in Emit does I/O. BenchmarkBroadcastEmit (telemetry package) is the
// in-process bus floor these numbers are read against; run both with
//
//	go test ./telemetry/... -bench 'BenchmarkChainEmit|BenchmarkBroadcastEmit|BenchmarkSegmentWriter' -benchmem -count=5

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"codeburg.org/lexbit/relurpify/telemetry"
)

func defaultBenchContext() context.Context { return context.Background() }

// benchEvent builds a representative event with realistic metadata weight.
func benchEvent(lifecycle bool, seq int) telemetry.Event {
	ev := telemetry.Event{
		Type:      telemetry.EventStateChange,
		Timestamp: time.Now().UTC(),
		SessionID: "bench-session",
		RunID:     "bench-run",
		AgentID:   "euclo",
		NodeID:    "node-" + strconv.Itoa(seq%8),
		TaskID:    "bench-task",
		Message:   "benchmark event " + strconv.Itoa(seq),
		Metadata: map[string]any{
			"tool":     "shell",
			"duration": 42,
			"seq":      seq,
			"success":  true,
			"req_id":   fmt.Sprintf("req-%d", seq),
			"scope":    map[string]any{"depth": 2},
		},
	}
	if lifecycle {
		ev.Type = telemetry.EventToolCall
		ev.Metadata["span_id"] = fmt.Sprintf("%016x", seq)
	}
	return ev
}

// benchPool pre-builds events so the loop measures Emit only — the same
// contract as BenchmarkBroadcastEmit, the bus floor.
func benchPool(b *testing.B, n int) []telemetry.Event {
	b.Helper()
	pool := make([]telemetry.Event, n)
	for i := range pool {
		pool[i] = benchEvent(i%4 == 0, i)
	}
	return pool
}

// BenchmarkChainEmitFileOnly measures Emit against a file-only chain (the
// offline default): conversion + routing, no network.
func BenchmarkChainEmitFileOnly(b *testing.B) {
	chain, err := New(defaultBenchContext(), Settings{
		Enabled:     true,
		FileEnabled: true,
		FileDir:     b.TempDir(),
	})
	if err != nil {
		b.Fatalf("new chain: %v", err)
	}
	defer func() { _ = chain.Close(defaultBenchContext()) }()

	pool := benchPool(b, 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chain.Emit(pool[i&1023])
	}
}

// BenchmarkChainEmitMixed measures the lifecycle/point split the runtime
// actually produces (roughly one lifecycle start/finish per ten point events).
func BenchmarkChainEmitMixed(b *testing.B) {
	chain, err := New(defaultBenchContext(), Settings{
		Enabled:     true,
		FileEnabled: true,
		FileDir:     b.TempDir(),
	})
	if err != nil {
		b.Fatalf("new chain: %v", err)
	}
	defer func() { _ = chain.Close(defaultBenchContext()) }()

	pool := benchPool(b, 1024)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ev := pool[i&1023]
		switch i % 10 {
		case 0:
			ev.Type = telemetry.EventToolCall
		case 5:
			ev.Type = telemetry.EventToolResult
		default:
			ev.Type = telemetry.EventStateChange
		}
		chain.Emit(ev)
	}
}

// BenchmarkSegmentWriter isolates the file leg's durability boundary: one
// complete OTLP-JSON batch line per write, rotation and pruning included.
func BenchmarkSegmentWriter(b *testing.B) {
	w := newSegmentWriter(b.TempDir(), segmentPrefix, "", 168*time.Hour, 256, nil)
	defer func() { _ = w.Close() }()

	line := []byte(`{"resourceLogs":{"scopeLogs":[{"logRecords":[{"timeUnixNano":"1760340000000000000","eventName":"bench.event","attributes":[{"key":"tool","value":{"stringValue":"shell"}}]}]}]}}` + "\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := w.Write(line); err != nil {
			b.Fatalf("write: %v", err)
		}
	}
}
