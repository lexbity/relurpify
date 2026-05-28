package telemetry

import (
	"testing"
	"time"
)

func TestMetricsCallsTotalIncrementOnSuccess(t *testing.T) {
	m := NewToolCallMetrics()
	m.RecordCall(true, 10*time.Millisecond, false)
	snap := m.Snapshot()
	if snap["calls_total"].(int64) != 1 {
		t.Fatalf("expected calls_total=1, got %v", snap["calls_total"])
	}
	if snap["calls_failed"].(int64) != 0 {
		t.Fatalf("expected calls_failed=0, got %v", snap["calls_failed"])
	}
}

func TestMetricsCallsTotalIncrementOnFailure(t *testing.T) {
	m := NewToolCallMetrics()
	m.RecordCall(false, 5*time.Millisecond, false)
	snap := m.Snapshot()
	if snap["calls_total"].(int64) != 1 {
		t.Fatalf("expected calls_total=1, got %v", snap["calls_total"])
	}
	if snap["calls_failed"].(int64) != 1 {
		t.Fatalf("expected calls_failed=1, got %v", snap["calls_failed"])
	}
}

func TestMetricsDurationObserved(t *testing.T) {
	m := NewToolCallMetrics()
	m.RecordCall(true, 100*time.Millisecond, false)
	snap := m.Snapshot()
	avg := snap["avg_duration_ns"].(int64)
	if avg < 90_000_000 || avg > 110_000_000 {
		t.Fatalf("expected avg_duration_ns ~100ms, got %d", avg)
	}
}

func TestMetricsTruncationCounterIncrements(t *testing.T) {
	m := NewToolCallMetrics()
	m.RecordCall(true, 0, true)
	snap := m.Snapshot()
	if snap["truncations_total"].(int64) != 1 {
		t.Fatalf("expected truncations_total=1, got %v", snap["truncations_total"])
	}
}

func TestMetricsNilMetricsIsNoop(t *testing.T) {
	var m *ToolCallMetrics
	m.RecordCall(true, 0, false)
	m.RecordDenial()
	m.RecordDoomLoop()
	snap := m.Snapshot()
	if snap != nil {
		t.Fatal("expected nil snapshot from nil metrics")
	}
}

func TestMetricsDenialCounter(t *testing.T) {
	m := NewToolCallMetrics()
	m.RecordDenial()
	m.RecordDenial()
	snap := m.Snapshot()
	if snap["denials_total"].(int64) != 2 {
		t.Fatalf("expected denials_total=2, got %v", snap["denials_total"])
	}
}

func TestMetricsDoomLoopCounter(t *testing.T) {
	m := NewToolCallMetrics()
	m.RecordDoomLoop()
	snap := m.Snapshot()
	if snap["doom_loops_total"].(int64) != 1 {
		t.Fatalf("expected doom_loops_total=1, got %v", snap["doom_loops_total"])
	}
}

func TestMetricsSuccessRate(t *testing.T) {
	m := NewToolCallMetrics()
	m.RecordCall(true, 0, false)
	m.RecordCall(false, 0, false)
	m.RecordCall(true, 0, false)
	snap := m.Snapshot()
	rate := snap["success_rate"].(float64)
	if rate < 0.66 || rate > 0.67 {
		t.Fatalf("expected success_rate ~0.667, got %f", rate)
	}
}
