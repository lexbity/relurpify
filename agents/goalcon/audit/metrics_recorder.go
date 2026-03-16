package audit

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"time"

	"github.com/lexcodex/relurpify/framework/core"
	"github.com/lexcodex/relurpify/framework/memory"
)

// ExecutionMetrics captures timing and success information for a single execution.
type ExecutionMetrics struct {
	OperatorName  string
	Success       bool
	Duration      time.Duration
	Depth         int
	ErrorMessage  string
}

// types.MetricsRecorder tracks execution outcomes and persists them to memory.
type types.MetricsRecorder struct {
	store           memory.MemoryStore
	metrics         OperatorMetricsCollection
	autoSave        bool
	saveInterval    int // Save after N recordings
	recordingCount  int
}

// NewMetricsRecorder creates a new metrics recorder.
func NewMetricsRecorder(store memory.MemoryStore) *types.MetricsRecorder {
	return &types.MetricsRecorder{
		store:        store,
		metrics:      make(OperatorMetricsCollection),
		autoSave:     true,
		saveInterval: 10, // Save every 10 recordings
	}
}

// LoadExisting loads previously persisted metrics from memory.
func (r *types.MetricsRecorder) LoadExisting() error {
	if r == nil || r.store == nil {
		return nil
	}
	r.metrics = LoadMetricsFromMemory(r.store)
	return nil
}

// RecordExecution adds a new execution result to the metrics.
func (r *types.MetricsRecorder) RecordExecution(execMetrics ExecutionMetrics) error {
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
func (r *types.MetricsRecorder) RecordOperatorExecution(operatorName string, success bool, duration time.Duration) error {
	return r.RecordExecution(ExecutionMetrics{
		OperatorName: operatorName,
		Success:      success,
		Duration:     duration,
	})
}

// GetMetrics returns the current metrics for an operator.
func (r *types.MetricsRecorder) GetMetrics(operatorName string) *OperatorMetrics {
	if r == nil || r.metrics == nil {
		return nil
	}
	return r.metrics[operatorName]
}

// GetAllMetrics returns a copy of all collected metrics.
func (r *types.MetricsRecorder) GetAllMetrics() OperatorMetricsCollection {
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
func (r *types.MetricsRecorder) Save() error {
	if r == nil || r.store == nil {
		return nil
	}
	return SaveMetricsToMemory(r.store, r.metrics)
}

// SetAutoSave configures automatic saving behavior.
func (r *types.MetricsRecorder) SetAutoSave(enabled bool, interval int) {
	if r == nil {
		return
	}
	r.autoSave = enabled
	if interval > 0 {
		r.saveInterval = interval
	}
}

// Snapshot returns a read-only metrics snapshot.
func (r *types.MetricsRecorder) Snapshot() MetricsSnapshot {
	if r == nil || r.metrics == nil {
		return MetricsSnapshot{}
	}
	return r.metrics.Snapshot()
}

// EstimateOperatorQuality provides a weighted score for operator selection.
// Higher score = more likely to be selected.
// Formula: (success_rate * 0.7) + (1.0 - normalized_duration * 0.3)
func (r *types.MetricsRecorder) EstimateOperatorQuality(operatorName string) float64 {
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
func (r *types.MetricsRecorder) ComparatorByQuality() func(op1, op2 *types.Operator) bool {
	return func(op1, op2 *types.Operator) bool {
		if op1 == nil || op2 == nil {
			return false
		}
		return r.EstimateOperatorQuality(op1.Name) > r.EstimateOperatorQuality(op2.Name)
	}
}

// RecordPlanExecution records metrics for a complete plan execution.
// Called after solving and executing a plan.
func (r *types.MetricsRecorder) RecordPlanExecution(plan *core.Plan, result *core.Result, duration time.Duration) error {
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
func (r *types.MetricsRecorder) ResetMetrics() {
	if r == nil {
		return
	}
	r.metrics = make(OperatorMetricsCollection)
	r.recordingCount = 0
}
