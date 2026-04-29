package operators

import (
	"encoding/json"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
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
func LoadMetricsFromMemory(store *memory.WorkingMemoryStore) OperatorMetricsCollection {
	if store == nil {
		return make(OperatorMetricsCollection)
	}
	record, ok := store.Scope("goalcon").Get("goalcon.operator_metrics")
	if !ok {
		return make(OperatorMetricsCollection)
	}
	jsonStr, ok := record.Value.(string)
	if !ok {
		return make(OperatorMetricsCollection)
	}
	var metrics OperatorMetricsCollection
	if err := json.Unmarshal([]byte(jsonStr), &metrics); err != nil {
		return make(OperatorMetricsCollection)
	}
	return metrics
}

// SaveMetricsToMemory persists operator metrics to the memory store.
func SaveMetricsToMemory(store *memory.WorkingMemoryStore, metrics OperatorMetricsCollection) error {
	if store == nil || metrics == nil {
		return nil
	}
	data, err := json.Marshal(metrics)
	if err != nil {
		return err
	}
	store.Scope("goalcon").Set("goalcon.operator_metrics", string(data), core.MemoryClassWorking)
	return nil
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
