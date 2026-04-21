package audit

import (
	"context"
	"encoding/json"
	"time"

	"codeburg.org/lexbit/relurpify/framework/core"
	"codeburg.org/lexbit/relurpify/framework/memory"
)

// ExecutionMetrics captures timing and success information for a single execution.
type ExecutionMetrics struct {
	OperatorName string
	Success      bool
	Duration     time.Duration
	Depth        int
	ErrorMessage string
}

// OperatorMetrics tracks aggregated execution statistics for a specific operator.
type OperatorMetrics struct {
	Name              string
	TotalExecutions   int
	SuccessfulCount   int
	FailedCount       int
	AvgDuration       time.Duration
	MinDuration       time.Duration
	MaxDuration       time.Duration
	SuccessRate       float64
	LastExecutionTime time.Time
}

// RecordExecution updates metrics for a single execution.
func (m *OperatorMetrics) RecordExecution(success bool, duration time.Duration) {
	if m == nil {
		return
	}
	m.TotalExecutions++
	if success {
		m.SuccessfulCount++
	} else {
		m.FailedCount++
	}

	// Update durations
	if m.AvgDuration == 0 {
		m.AvgDuration = duration
		m.MinDuration = duration
		m.MaxDuration = duration
	} else {
		// Running average
		m.AvgDuration = (m.AvgDuration*time.Duration(m.TotalExecutions-1) + duration) / time.Duration(m.TotalExecutions)
		if duration < m.MinDuration {
			m.MinDuration = duration
		}
		if duration > m.MaxDuration {
			m.MaxDuration = duration
		}
	}

	// Update success rate
	if m.TotalExecutions > 0 {
		m.SuccessRate = float64(m.SuccessfulCount) / float64(m.TotalExecutions)
	}
	m.LastExecutionTime = time.Now()
}

// SuccessRateOrDefault returns SuccessRate, or defaultRate when nil or no executions.
func (m *OperatorMetrics) SuccessRateOrDefault(defaultRate float64) float64 {
	if m == nil || m.TotalExecutions == 0 {
		return defaultRate
	}
	return m.SuccessRate
}

// OperatorMetricsCollection is a map of operator names to their metrics.
type OperatorMetricsCollection map[string]*OperatorMetrics

// GetOrCreateMetrics retrieves or creates metrics for an operator.
func (c OperatorMetricsCollection) GetOrCreateMetrics(operatorName string) *OperatorMetrics {
	if c == nil {
		return nil
	}
	if m, exists := c[operatorName]; exists {
		return m
	}
	m := &OperatorMetrics{Name: operatorName}
	c[operatorName] = m
	return m
}

// Snapshot returns a read-only snapshot of the metrics.
func (c OperatorMetricsCollection) Snapshot() MetricsSnapshot {
	snapshot := MetricsSnapshot{
		Operators: make(map[string]OperatorMetrics),
	}
	for name, m := range c {
		if m != nil {
			snapshot.Operators[name] = *m
		}
	}
	return snapshot
}

// MetricsSnapshot is a read-only snapshot of collected metrics.
type MetricsSnapshot struct {
	Operators    map[string]OperatorMetrics
	SnapshotTime time.Time
}

const metricsMemoryKey = "goalcon.operator_metrics"

// LoadMetricsFromMemory loads previously persisted metrics from the memory store.
func LoadMetricsFromMemory(store memory.MemoryStore) OperatorMetricsCollection {
	if store == nil {
		return make(OperatorMetricsCollection)
	}
	record, ok, err := store.Recall(context.Background(), metricsMemoryKey, memory.MemoryScopeProject)
	if err != nil || !ok || record == nil {
		return make(OperatorMetricsCollection)
	}
	raw, ok := record.Value["metrics_json"]
	if !ok {
		return make(OperatorMetricsCollection)
	}
	jsonStr, ok := raw.(string)
	if !ok {
		return make(OperatorMetricsCollection)
	}
	var collection OperatorMetricsCollection
	if err := json.Unmarshal([]byte(jsonStr), &collection); err != nil {
		return make(OperatorMetricsCollection)
	}
	return collection
}

// SaveMetricsToMemory persists metrics to the memory store.
func SaveMetricsToMemory(store memory.MemoryStore, metrics OperatorMetricsCollection) error {
	if store == nil {
		return nil
	}
	data, err := json.Marshal(metrics)
	if err != nil {
		return err
	}
	return store.Remember(context.Background(), metricsMemoryKey, map[string]interface{}{
		"metrics_json": string(data),
	}, memory.MemoryScopeProject)
}

// MetricsRecorder tracks execution outcomes and persists them to memory.
type MetricsRecorder struct {
	store          memory.MemoryStore
	metrics        OperatorMetricsCollection
	autoSave       bool
	saveInterval   int // Save after N recordings
	recordingCount int
}

// NewMetricsRecorder creates a new metrics recorder.
func NewMetricsRecorder(store memory.MemoryStore) *MetricsRecorder {
	return &MetricsRecorder{
		store:        store,
		metrics:      make(OperatorMetricsCollection),
		autoSave:     true,
		saveInterval: 10, // Save every 10 recordings
	}
}

// LoadExisting loads previously persisted metrics from memory.
func (r *MetricsRecorder) LoadExisting() error {
	if r == nil || r.store == nil {
		return nil
	}
	r.metrics = LoadMetricsFromMemory(r.store)
	return nil
}

// RecordExecution adds a new execution result to the metrics.
func (r *MetricsRecorder) RecordExecution(execMetrics ExecutionMetrics) error {
	if r == nil || r.metrics == nil {
		return nil
	}

	// Get or create metrics for this operator
	opMetrics := r.metrics.GetOrCreateMetrics(execMetrics.OperatorName)
	if opMetrics == nil {
		return nil
	}

	// Record the execution
	opMetrics.RecordExecution(execMetrics.Success, execMetrics.Duration)

	r.recordingCount++

	// Auto-save on interval
	if r.autoSave && r.recordingCount >= r.saveInterval {
		if err := r.Save(); err != nil {
			// Log error, but don't fail the agent
			return nil
		}
		r.recordingCount = 0
	}

	return nil
}

// RecordOperatorExecution is a convenience for recording a single operator result.
func (r *MetricsRecorder) RecordOperatorExecution(operatorName string, success bool, duration time.Duration) error {
	return r.RecordExecution(ExecutionMetrics{
		OperatorName: operatorName,
		Success:      success,
		Duration:     duration,
	})
}

// GetMetrics returns the current metrics for an operator.
func (r *MetricsRecorder) GetMetrics(operatorName string) *OperatorMetrics {
	if r == nil || r.metrics == nil {
		return nil
	}
	return r.metrics[operatorName]
}

// GetAllMetrics returns a copy of all collected metrics.
func (r *MetricsRecorder) GetAllMetrics() OperatorMetricsCollection {
	if r == nil || r.metrics == nil {
		return make(OperatorMetricsCollection)
	}
	// Return shallow copy
	result := make(OperatorMetricsCollection)
	for k, v := range r.metrics {
		result[k] = v
	}
	return result
}

// Save persists metrics to memory store.
func (r *MetricsRecorder) Save() error {
	if r == nil || r.store == nil {
		return nil
	}
	return SaveMetricsToMemory(r.store, r.metrics)
}

// SetAutoSave configures automatic saving behavior.
func (r *MetricsRecorder) SetAutoSave(enabled bool, interval int) {
	if r == nil {
		return
	}
	r.autoSave = enabled
	if interval > 0 {
		r.saveInterval = interval
	}
}

// Snapshot returns a read-only metrics snapshot.
func (r *MetricsRecorder) Snapshot() MetricsSnapshot {
	if r == nil || r.metrics == nil {
		return MetricsSnapshot{}
	}
	return r.metrics.Snapshot()
}

// EstimateOperatorQuality provides a weighted score for operator selection.
// Higher score = more likely to be selected.
// Formula: (success_rate * 0.7) + (1.0 - normalized_duration * 0.3)
func (r *MetricsRecorder) EstimateOperatorQuality(operatorName string) float64 {
	if r == nil || r.metrics == nil {
		return 1.0 // Default score for unknown operators
	}

	m := r.metrics[operatorName]
	if m == nil || m.TotalExecutions == 0 {
		return 1.0 // No data = neutral score
	}

	// Success rate component (0-1)
	successScore := m.SuccessRate * 0.7

	// Duration component: normalize to 1.0-based scale
	// Favor faster operators, but don't penalize unknown
	var durationScore float64
	if m.AvgDuration > 0 {
		// Assume typical operator takes ~100ms
		// Slower operators get lower scores
		normalized := float64(m.AvgDuration.Milliseconds()) / 100.0
		if normalized > 1.0 {
			normalized = 1.0
		}
		durationScore = (1.0 - normalized) * 0.3
	} else {
		durationScore = 0.3 // Assume best case
	}

	return successScore + durationScore
}

// ComparatorByQuality returns a comparator function for sorting operators by estimated quality.
func (r *MetricsRecorder) ComparatorByQuality() func(op1, op2 interface{}) bool {
	return func(op1, op2 interface{}) bool {
		// Simple comparator placeholder - can be customized based on operator type
		return false
	}
}

// RecordPlanExecution records metrics for a complete plan execution.
// Called after solving and executing a plan.
func (r *MetricsRecorder) RecordPlanExecution(plan *core.Plan, result *core.Result, duration time.Duration) error {
	if r == nil || plan == nil || result == nil {
		return nil
	}

	// Record success/failure for plan as a whole
	for _, step := range plan.Steps {
		// Approximate: if plan succeeded, assume all steps succeeded
		// In Phase 6, this will be more granular
		success := result.Success
		stepDuration := duration / time.Duration(len(plan.Steps))
		if err := r.RecordOperatorExecution(step.Tool, success, stepDuration); err != nil {
			return err
		}
	}

	// Auto-persist after plan
	return r.Save()
}

// ResetMetrics clears all collected metrics (for testing).
func (r *MetricsRecorder) ResetMetrics() {
	if r == nil {
		return
	}
	r.metrics = make(OperatorMetricsCollection)
	r.recordingCount = 0
}
