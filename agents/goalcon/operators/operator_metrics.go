package operators

import (
	"context"
	"encoding/json"
	"time"

	"github.com/lexcodex/relurpify/framework/memory"
)

// OperatorMetrics tracks execution statistics for a single operator.
type OperatorMetrics struct {
	Name            string        `json:"name"`
	SuccessCount    int           `json:"success_count"`
	FailureCount    int           `json:"failure_count"`
	TotalExecutions int           `json:"total_executions"`
	TotalDuration   time.Duration `json:"total_duration"`
	AvgDuration     time.Duration `json:"avg_duration"`
	SuccessRate     float64       `json:"success_rate"`
	LastExecuted    time.Time     `json:"last_executed,omitempty"`
}

// OperatorMetricsCollection maps operator names to their metrics.
type OperatorMetricsCollection map[string]*OperatorMetrics

// RecordExecution updates metrics with a new execution result.
func (m *OperatorMetrics) RecordExecution(success bool, duration time.Duration) {
	if m == nil {
		return
	}
	m.TotalExecutions++
	m.TotalDuration += duration
	m.AvgDuration = m.TotalDuration / time.Duration(m.TotalExecutions)
	m.LastExecuted = time.Now()

	if success {
		m.SuccessCount++
	} else {
		m.FailureCount++
	}

	if m.TotalExecutions > 0 {
		m.SuccessRate = float64(m.SuccessCount) / float64(m.TotalExecutions)
	}
}

// SuccessRateOrDefault returns the success rate, or a default if no executions.
func (m *OperatorMetrics) SuccessRateOrDefault(defaultRate float64) float64 {
	if m == nil || m.TotalExecutions == 0 {
		return defaultRate
	}
	return m.SuccessRate
}

// GetOrCreateMetrics gets existing metrics or creates new ones.
func (c OperatorMetricsCollection) GetOrCreateMetrics(name string) *OperatorMetrics {
	if c == nil {
		return nil
	}
	if m, ok := c[name]; ok {
		return m
	}
	m := &OperatorMetrics{Name: name}
	c[name] = m
	return m
}

// LoadMetricsFromMemory retrieves operator metrics from the memory store.
func LoadMetricsFromMemory(store memory.MemoryStore) OperatorMetricsCollection {
	if store == nil {
		return make(OperatorMetricsCollection)
	}

	// Retrieve persisted metrics from project scope
	record, exists, err := store.Recall(context.Background(), "goalcon.operator_metrics", memory.MemoryScopeProject)
	if err != nil || !exists || record == nil {
		return make(OperatorMetricsCollection)
	}

	// Parse JSON back to metrics collection
	// The Value field is map[string]interface{}, so we need to re-marshal it
	var metrics OperatorMetricsCollection
	jsonBytes, err := json.Marshal(record.Value)
	if err != nil {
		return make(OperatorMetricsCollection)
	}

	if err := json.Unmarshal(jsonBytes, &metrics); err != nil {
		return make(OperatorMetricsCollection)
	}

	if metrics == nil {
		metrics = make(OperatorMetricsCollection)
	}
	return metrics
}

// SaveMetricsToMemory persists operator metrics to the memory store.
func SaveMetricsToMemory(store memory.MemoryStore, metrics OperatorMetricsCollection) error {
	if store == nil || metrics == nil {
		return nil
	}

	// Convert metrics to map[string]interface{} for MemoryStore
	metricsMap := make(map[string]interface{})
	for k, v := range metrics {
		metricsMap[k] = v
	}

	// Store in project scope (persists across sessions)
	return store.Remember(context.Background(), "goalcon.operator_metrics", metricsMap, memory.MemoryScopeProject)
}

// MetricsSnapshot provides a read-only view of metrics at a point in time.
type MetricsSnapshot struct {
	TotalOperators     int
	TotalExecutions    int
	AverageSuccessRate float64
	MetricsPerOperator map[string]*OperatorMetrics
}

// Snapshot creates a read-only view of current metrics.
func (c OperatorMetricsCollection) Snapshot() MetricsSnapshot {
	if c == nil {
		return MetricsSnapshot{}
	}

	snap := MetricsSnapshot{
		TotalOperators:     len(c),
		MetricsPerOperator: make(map[string]*OperatorMetrics),
	}

	totalSuccessRate := 0.0
	for name, m := range c {
		snap.MetricsPerOperator[name] = m
		snap.TotalExecutions += m.TotalExecutions
		totalSuccessRate += m.SuccessRate
	}

	if snap.TotalOperators > 0 {
		snap.AverageSuccessRate = totalSuccessRate / float64(snap.TotalOperators)
	}

	return snap
}
