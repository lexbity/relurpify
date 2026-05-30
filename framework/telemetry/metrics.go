package telemetry

import (
	"sync/atomic"
	"time"
)

// ToolCallMetrics aggregates tool-call observability counters. All fields
// are safe for concurrent access. A nil *ToolCallMetrics is a valid no-op.
type ToolCallMetrics struct {
	callsTotal       atomic.Int64
	callsFailed      atomic.Int64
	truncationsTotal atomic.Int64
	denialsTotal     atomic.Int64
	doomLoopsTotal   atomic.Int64

	// Accumulated duration in nanoseconds for computing averages.
	durationTotalNs atomic.Int64
}

// NewToolCallMetrics initialises and returns a metrics collector.
func NewToolCallMetrics() *ToolCallMetrics {
	return &ToolCallMetrics{}
}

// RecordCall records a single tool invocation outcome.
func (m *ToolCallMetrics) RecordCall(success bool, duration time.Duration) {
	if m == nil {
		return
	}
	m.callsTotal.Add(1)
	if !success {
		m.callsFailed.Add(1)
	}
	m.durationTotalNs.Add(duration.Nanoseconds())
}

// RecordDenial records a permission denial.
func (m *ToolCallMetrics) RecordDenial() {
	if m == nil {
		return
	}
	m.denialsTotal.Add(1)
}

// RecordDoomLoop records a doom loop detection.
func (m *ToolCallMetrics) RecordDoomLoop() {
	if m == nil {
		return
	}
	m.doomLoopsTotal.Add(1)
}

// Snapshot returns the current counter values as a map for telemetry export.
func (m *ToolCallMetrics) Snapshot() map[string]any {
	if m == nil {
		return nil
	}
	total := m.callsTotal.Load()
	failed := m.callsFailed.Load()
	avgDuration := time.Duration(0)
	if total > 0 {
		avgDuration = time.Duration(m.durationTotalNs.Load() / total)
	}
	return map[string]any{
		"calls_total":       total,
		"calls_failed":      failed,
		"success_rate":      float64(total-failed) / float64(max(total, 1)),
		"truncations_total": m.truncationsTotal.Load(),
		"denials_total":     m.denialsTotal.Load(),
		"doom_loops_total":  m.doomLoopsTotal.Load(),
		"avg_duration_ns":   avgDuration.Nanoseconds(),
	}
}
