package audit

import (
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/core"
)

func TestOperatorMetrics_RecordExecution(t *testing.T) {
	m := &OperatorMetrics{Name: "TestOp"}

	// Record successful execution
	m.RecordExecution(true, 100*time.Millisecond)
	if m.TotalExecutions != 1 {
		t.Errorf("expected 1 execution, got %d", m.TotalExecutions)
	}
	if m.SuccessfulCount != 1 {
		t.Errorf("expected 1 success, got %d", m.SuccessfulCount)
	}
	if m.FailedCount != 0 {
		t.Errorf("expected 0 failures, got %d", m.FailedCount)
	}

	// Record failed execution
	m.RecordExecution(false, 200*time.Millisecond)
	if m.TotalExecutions != 2 {
		t.Errorf("expected 2 executions, got %d", m.TotalExecutions)
	}
	if m.SuccessfulCount != 1 {
		t.Errorf("expected 1 success, got %d", m.SuccessfulCount)
	}
	if m.FailedCount != 1 {
		t.Errorf("expected 1 failure, got %d", m.FailedCount)
	}

	// Check success rate
	if m.SuccessRate != 0.5 {
		t.Errorf("expected 0.5 success rate, got %f", m.SuccessRate)
	}
}

func TestOperatorMetrics_RecordExecution_Nil(t *testing.T) {
	var m *OperatorMetrics
	// Should not panic
	m.RecordExecution(true, 100*time.Millisecond)
}

func TestOperatorMetrics_SuccessRateOrDefault(t *testing.T) {
	// Test with nil metrics
	var m *OperatorMetrics
	rate := m.SuccessRateOrDefault(0.8)
	if rate != 0.8 {
		t.Errorf("expected default 0.8, got %f", rate)
	}

	// Test with no executions
	m = &OperatorMetrics{Name: "TestOp"}
	rate = m.SuccessRateOrDefault(0.9)
	if rate != 0.9 {
		t.Errorf("expected default 0.9, got %f", rate)
	}

	// Test with executions
	m.RecordExecution(true, 100*time.Millisecond)
	m.RecordExecution(false, 100*time.Millisecond)
	rate = m.SuccessRateOrDefault(0.9)
	if rate != 0.5 {
		t.Errorf("expected 0.5, got %f", rate)
	}
}

func TestOperatorMetricsCollection_GetOrCreateMetrics(t *testing.T) {
	c := make(OperatorMetricsCollection)

	// Get new metrics
	m1 := c.GetOrCreateMetrics("Op1")
	if m1 == nil {
		t.Fatal("expected non-nil metrics")
	}
	if m1.Name != "Op1" {
		t.Errorf("expected name Op1, got %s", m1.Name)
	}

	// Get existing metrics
	m1.RecordExecution(true, 100*time.Millisecond)
	m2 := c.GetOrCreateMetrics("Op1")
	if m2.TotalExecutions != 1 {
		t.Errorf("expected 1 execution from existing metrics, got %d", m2.TotalExecutions)
	}
}

func TestOperatorMetricsCollection_GetOrCreateMetrics_Nil(t *testing.T) {
	var c OperatorMetricsCollection
	m := c.GetOrCreateMetrics("Op1")
	if m != nil {
		t.Error("expected nil for nil collection")
	}
}

func TestOperatorMetricsCollection_Snapshot(t *testing.T) {
	c := make(OperatorMetricsCollection)
	c.GetOrCreateMetrics("Op1").RecordExecution(true, 100*time.Millisecond)
	c.GetOrCreateMetrics("Op2").RecordExecution(false, 200*time.Millisecond)

	snapshot := c.Snapshot()
	if len(snapshot.Operators) != 2 {
		t.Errorf("expected 2 operators in snapshot, got %d", len(snapshot.Operators))
	}

	// Check that snapshot contains the data
	if op1, ok := snapshot.Operators["Op1"]; !ok {
		t.Error("expected Op1 in snapshot")
	} else if op1.SuccessfulCount != 1 {
		t.Errorf("expected Op1 to have 1 success, got %d", op1.SuccessfulCount)
	}
}

func TestOperatorMetricsCollection_Snapshot_Nil(t *testing.T) {
	var c OperatorMetricsCollection
	snapshot := c.Snapshot()
	if len(snapshot.Operators) != 0 {
		t.Errorf("expected 0 operators, got %d", len(snapshot.Operators))
	}
}

func TestNewMetricsRecorder(t *testing.T) {
	recorder := NewMetricsRecorder(nil)
	if recorder == nil {
		t.Fatal("expected non-nil recorder")
	}
	if recorder.store != nil {
		t.Error("expected nil store")
	}
	if recorder.metrics == nil {
		t.Error("expected metrics to be initialized")
	}
	if !recorder.autoSave {
		t.Error("expected autoSave to be true")
	}
	if recorder.saveInterval != 10 {
		t.Errorf("expected saveInterval 10, got %d", recorder.saveInterval)
	}
}

func TestMetricsRecorder_LoadExisting(t *testing.T) {
	// Test with nil recorder
	var r *MetricsRecorder
	err := r.LoadExisting()
	if err != nil {
		t.Errorf("expected no error for nil recorder, got %v", err)
	}

	// Test with nil store
	r = NewMetricsRecorder(nil)
	err = r.LoadExisting()
	if err != nil {
		t.Errorf("expected no error for nil store, got %v", err)
	}
}

func TestMetricsRecorder_RecordExecution(t *testing.T) {
	recorder := NewMetricsRecorder(nil)

	execMetrics := ExecutionMetrics{
		OperatorName: "TestOp",
		Success:      true,
		Duration:     100 * time.Millisecond,
		Depth:        1,
	}

	err := recorder.RecordExecution(execMetrics)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Check metrics were recorded
	m := recorder.GetMetrics("TestOp")
	if m == nil {
		t.Fatal("expected metrics for TestOp")
	}
	if m.TotalExecutions != 1 {
		t.Errorf("expected 1 execution, got %d", m.TotalExecutions)
	}
}

func TestMetricsRecorder_RecordExecution_Nil(t *testing.T) {
	var r *MetricsRecorder
	err := r.RecordExecution(ExecutionMetrics{OperatorName: "TestOp"})
	if err != nil {
		t.Errorf("expected no error for nil recorder, got %v", err)
	}
}

func TestMetricsRecorder_RecordOperatorExecution(t *testing.T) {
	recorder := NewMetricsRecorder(nil)

	err := recorder.RecordOperatorExecution("TestOp", true, 100*time.Millisecond)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	m := recorder.GetMetrics("TestOp")
	if m == nil || m.TotalExecutions != 1 {
		t.Error("expected metrics to be recorded")
	}
}

func TestMetricsRecorder_GetMetrics(t *testing.T) {
	recorder := NewMetricsRecorder(nil)

	// Get non-existent metrics
	m := recorder.GetMetrics("NonExistent")
	if m != nil {
		t.Error("expected nil for non-existent operator")
	}

	// Record and get
	recorder.RecordOperatorExecution("TestOp", true, 100*time.Millisecond)
	m = recorder.GetMetrics("TestOp")
	if m == nil {
		t.Error("expected metrics for TestOp")
	}
}

func TestMetricsRecorder_GetMetrics_Nil(t *testing.T) {
	var r *MetricsRecorder
	m := r.GetMetrics("TestOp")
	if m != nil {
		t.Error("expected nil for nil recorder")
	}
}

func TestMetricsRecorder_GetAllMetrics(t *testing.T) {
	recorder := NewMetricsRecorder(nil)
	recorder.RecordOperatorExecution("Op1", true, 100*time.Millisecond)
	recorder.RecordOperatorExecution("Op2", false, 200*time.Millisecond)

	all := recorder.GetAllMetrics()
	if len(all) != 2 {
		t.Errorf("expected 2 operators, got %d", len(all))
	}
}

func TestMetricsRecorder_GetAllMetrics_Nil(t *testing.T) {
	var r *MetricsRecorder
	all := r.GetAllMetrics()
	if len(all) != 0 {
		t.Errorf("expected 0 operators, got %d", len(all))
	}
}

func TestMetricsRecorder_Save(t *testing.T) {
	// Test with nil recorder
	var r *MetricsRecorder
	err := r.Save()
	if err != nil {
		t.Errorf("expected no error for nil recorder, got %v", err)
	}

	// Test with nil store
	r = NewMetricsRecorder(nil)
	err = r.Save()
	if err != nil {
		t.Errorf("expected no error for nil store, got %v", err)
	}
}

func TestMetricsRecorder_SetAutoSave(t *testing.T) {
	recorder := NewMetricsRecorder(nil)

	recorder.SetAutoSave(false, 20)
	if recorder.autoSave {
		t.Error("expected autoSave to be false")
	}
	if recorder.saveInterval != 20 {
		t.Errorf("expected saveInterval 20, got %d", recorder.saveInterval)
	}
}

func TestMetricsRecorder_SetAutoSave_Nil(t *testing.T) {
	var r *MetricsRecorder
	// Should not panic
	r.SetAutoSave(false, 20)
}

func TestMetricsRecorder_Snapshot(t *testing.T) {
	recorder := NewMetricsRecorder(nil)
	recorder.RecordOperatorExecution("Op1", true, 100*time.Millisecond)

	snapshot := recorder.Snapshot()
	if len(snapshot.Operators) != 1 {
		t.Errorf("expected 1 operator, got %d", len(snapshot.Operators))
	}
}

func TestMetricsRecorder_Snapshot_Nil(t *testing.T) {
	var r *MetricsRecorder
	snapshot := r.Snapshot()
	if len(snapshot.Operators) != 0 {
		t.Errorf("expected 0 operators, got %d", len(snapshot.Operators))
	}
}

func TestMetricsRecorder_EstimateOperatorQuality(t *testing.T) {
	// Test with nil recorder
	var r *MetricsRecorder
	quality := r.EstimateOperatorQuality("TestOp")
	if quality != 1.0 {
		t.Errorf("expected 1.0 for nil recorder, got %f", quality)
	}

	// Test unknown operator
	r = NewMetricsRecorder(nil)
	quality = r.EstimateOperatorQuality("UnknownOp")
	if quality != 1.0 {
		t.Errorf("expected 1.0 for unknown operator, got %f", quality)
	}

	// Test operator with data
	r.RecordOperatorExecution("TestOp", true, 50*time.Millisecond)
	quality = r.EstimateOperatorQuality("TestOp")
	// Success rate 1.0 * 0.7 + duration score
	if quality <= 0.7 {
		t.Errorf("expected quality > 0.7 for perfect success, got %f", quality)
	}
}

func TestMetricsRecorder_ComparatorByQuality(t *testing.T) {
	recorder := NewMetricsRecorder(nil)
	comparator := recorder.ComparatorByQuality()

	// The comparator is a placeholder that always returns false
	result := comparator("op1", "op2")
	if result {
		t.Error("expected comparator to return false")
	}
}

func TestMetricsRecorder_RecordPlanExecution(t *testing.T) {
	recorder := NewMetricsRecorder(nil)

	plan := &core.Plan{
		Steps: []core.PlanStep{
			{ID: "step1", Tool: "Tool1"},
			{ID: "step2", Tool: "Tool2"},
		},
	}
	result := &core.Result{Success: true}
	duration := 200 * time.Millisecond

	err := recorder.RecordPlanExecution(plan, result, duration)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Check both tools were recorded
	if recorder.GetMetrics("Tool1") == nil {
		t.Error("expected metrics for Tool1")
	}
	if recorder.GetMetrics("Tool2") == nil {
		t.Error("expected metrics for Tool2")
	}
}

func TestMetricsRecorder_RecordPlanExecution_Nil(t *testing.T) {
	var r *MetricsRecorder
	err := r.RecordPlanExecution(nil, nil, 0)
	if err != nil {
		t.Errorf("expected no error for nil recorder, got %v", err)
	}
}

func TestMetricsRecorder_RecordPlanExecution_EmptyPlan(t *testing.T) {
	recorder := NewMetricsRecorder(nil)

	plan := &core.Plan{Steps: []core.PlanStep{}}
	result := &core.Result{Success: true}

	err := recorder.RecordPlanExecution(plan, result, 100*time.Millisecond)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestMetricsRecorder_ResetMetrics(t *testing.T) {
	recorder := NewMetricsRecorder(nil)
	recorder.RecordOperatorExecution("TestOp", true, 100*time.Millisecond)

	if recorder.GetMetrics("TestOp") == nil {
		t.Fatal("expected metrics before reset")
	}

	recorder.ResetMetrics()

	if recorder.GetMetrics("TestOp") != nil {
		t.Error("expected metrics to be cleared")
	}
	if recorder.recordingCount != 0 {
		t.Errorf("expected recordingCount 0, got %d", recorder.recordingCount)
	}
}

func TestMetricsRecorder_ResetMetrics_Nil(t *testing.T) {
	var r *MetricsRecorder
	// Should not panic
	r.ResetMetrics()
}

func TestLoadMetricsFromMemory_NilStore(t *testing.T) {
	metrics := LoadMetricsFromMemory(nil)
	if metrics == nil {
		t.Fatal("expected non-nil metrics")
	}
	if len(metrics) != 0 {
		t.Errorf("expected empty metrics, got %d", len(metrics))
	}
}

func TestSaveMetricsToMemory_NilStore(t *testing.T) {
	err := SaveMetricsToMemory(nil, make(OperatorMetricsCollection))
	if err != nil {
		t.Errorf("expected no error for nil store, got %v", err)
	}
}

func TestSaveMetricsToMemory_NilMetrics(t *testing.T) {
	err := SaveMetricsToMemory(nil, nil)
	if err != nil {
		t.Errorf("expected no error for nil metrics, got %v", err)
	}
}

func TestMetricsRecorder_AutoSaveTrigger(t *testing.T) {
	recorder := NewMetricsRecorder(nil)
	recorder.SetAutoSave(true, 2) // Save every 2 recordings

	// Record 2 executions - should trigger auto-save (but store is nil so no actual save)
	recorder.RecordOperatorExecution("Op1", true, 100*time.Millisecond)
	recorder.RecordOperatorExecution("Op2", true, 100*time.Millisecond)

	// If we got here without panic, auto-save with nil store works
	if recorder.recordingCount != 0 {
		// recordingCount resets after auto-save
		t.Errorf("expected recordingCount to be reset after auto-save, got %d", recorder.recordingCount)
	}
}

func TestMetricsSnapshot_ZeroValue(t *testing.T) {
	var snapshot MetricsSnapshot
	if len(snapshot.Operators) != 0 {
		t.Error("expected empty operators map")
	}
	if !snapshot.SnapshotTime.IsZero() {
		t.Error("expected zero snapshot time")
	}
}
