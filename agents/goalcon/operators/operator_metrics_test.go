package operators

import (
	"github.com/lexcodex/relurpify/agents/goalcon/types"
)

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lexcodex/relurpify/framework/memory"
)

func TestOperatorMetrics_RecordExecution(t *testing.T) {
	m := &goalcon.OperatorMetrics{Name: "TestOp"}

	m.RecordExecution(true, 100*time.Millisecond)
	if m.SuccessCount != 1 {
		t.Errorf("expected SuccessCount=1, got %d", m.SuccessCount)
	}
	if m.TotalExecutions != 1 {
		t.Errorf("expected TotalExecutions=1, got %d", m.TotalExecutions)
	}

	m.RecordExecution(false, 50*time.Millisecond)
	if m.FailureCount != 1 {
		t.Errorf("expected FailureCount=1, got %d", m.FailureCount)
	}
	if m.TotalExecutions != 2 {
		t.Errorf("expected TotalExecutions=2, got %d", m.TotalExecutions)
	}

	expectedRate := 0.5
	if m.SuccessRate != expectedRate {
		t.Errorf("expected SuccessRate=%.2f, got %.2f", expectedRate, m.SuccessRate)
	}

	expectedDuration := 75 * time.Millisecond
	if m.AvgDuration != expectedDuration {
		t.Errorf("expected AvgDuration=%v, got %v", expectedDuration, m.AvgDuration)
	}
}

func TestOperatorMetrics_SuccessRateOrDefault(t *testing.T) {
	tests := []struct {
		name        string
		metrics     *goalcon.OperatorMetrics
		defaultRate float64
		expected    float64
	}{
		{
			name:        "nil metrics",
			metrics:     nil,
			defaultRate: 0.9,
			expected:    0.9,
		},
		{
			name:        "no executions",
			metrics:     &goalcon.OperatorMetrics{Name: "op"},
			defaultRate: 0.75,
			expected:    0.75,
		},
		{
			name: "with executions",
			metrics: &goalcon.OperatorMetrics{
				Name:           "op",
				SuccessCount:   8,
				TotalExecutions: 10,
				SuccessRate:    0.8,
			},
			defaultRate: 0.5,
			expected:    0.8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.metrics.SuccessRateOrDefault(tt.defaultRate)
			if got != tt.expected {
				t.Errorf("expected %.2f, got %.2f", tt.expected, got)
			}
		})
	}
}

func TestOperatorMetricsCollection_GetOrCreate(t *testing.T) {
	collection := make(goalcon.OperatorMetricsCollection)

	m1 := collection.GetOrCreateMetrics("op1")
	if m1 == nil || m1.Name != "op1" {
		t.Fatal("expected created metrics for op1")
	}

	m1Again := collection.GetOrCreateMetrics("op1")
	if m1Again != m1 {
		t.Fatal("expected same metrics instance")
	}

	if len(collection) != 1 {
		t.Errorf("expected 1 entry, got %d", len(collection))
	}
}

func TestOperatorMetricsCollection_Snapshot(t *testing.T) {
	collection := make(goalcon.OperatorMetricsCollection)
	m1 := collection.GetOrCreateMetrics("op1")
	m1.RecordExecution(true, 50*time.Millisecond)
	m1.RecordExecution(true, 100*time.Millisecond)

	m2 := collection.GetOrCreateMetrics("op2")
	m2.RecordExecution(true, 75*time.Millisecond)
	m2.RecordExecution(false, 75*time.Millisecond)

	snap := collection.Snapshot()

	if snap.TotalOperators != 2 {
		t.Errorf("expected 2 operators, got %d", snap.TotalOperators)
	}
	if snap.TotalExecutions != 4 {
		t.Errorf("expected 4 total executions, got %d", snap.TotalExecutions)
	}
	if snap.AverageSuccessRate != 0.75 { // (1.0 + 0.5) / 2
		t.Errorf("expected avg success rate 0.75, got %.2f", snap.AverageSuccessRate)
	}
}

func TestMetricsRecorder_RecordExecution(t *testing.T) {
	recorder := goalcon.NewMetricsRecorder(nil) // No memory store

	recorder.RecordOperatorExecution("op1", true, 100*time.Millisecond)
	m := recorder.GetMetrics("op1")
	if m == nil || m.SuccessCount != 1 {
		t.Fatal("expected recorded execution")
	}

	recorder.RecordOperatorExecution("op1", false, 50*time.Millisecond)
	m = recorder.GetMetrics("op1")
	if m.FailureCount != 1 {
		t.Fatal("expected failure count=1")
	}
}

func TestMetricsRecorder_EstimateOperatorQuality(t *testing.T) {
	recorder := goalcon.NewMetricsRecorder(nil)

	// Unknown operator should get default score
	defaultScore := recorder.EstimateOperatorQuality("unknown")
	if defaultScore != 1.0 {
		t.Errorf("expected default score 1.0, got %.2f", defaultScore)
	}

	// Create operator with high success rate
	recorder.RecordOperatorExecution("good_op", true, 50*time.Millisecond)
	recorder.RecordOperatorExecution("good_op", true, 50*time.Millisecond)
	goodScore := recorder.EstimateOperatorQuality("good_op")

	// Create operator with low success rate
	recorder.RecordOperatorExecution("bad_op", false, 100*time.Millisecond)
	recorder.RecordOperatorExecution("bad_op", false, 100*time.Millisecond)
	badScore := recorder.EstimateOperatorQuality("bad_op")

	if goodScore <= badScore {
		t.Errorf("expected good_op (%.2f) > bad_op (%.2f)", goodScore, badScore)
	}
}

func TestMetricsRecorder_GetAllMetrics(t *testing.T) {
	recorder := goalcon.NewMetricsRecorder(nil)

	recorder.RecordOperatorExecution("op1", true, 100*time.Millisecond)
	recorder.RecordOperatorExecution("op2", true, 50*time.Millisecond)

	all := recorder.GetAllMetrics()
	if len(all) != 2 {
		t.Errorf("expected 2 operators, got %d", len(all))
	}
	if all["op1"] == nil || all["op2"] == nil {
		t.Fatal("expected both operators in metrics")
	}
}

func TestMetricsRecorder_Snapshot(t *testing.T) {
	recorder := goalcon.NewMetricsRecorder(nil)
	recorder.RecordOperatorExecution("op1", true, 100*time.Millisecond)
	recorder.RecordOperatorExecution("op1", false, 100*time.Millisecond)

	snap := recorder.Snapshot()
	if snap.TotalOperators != 1 {
		t.Errorf("expected 1 operator, got %d", snap.TotalOperators)
	}
	if snap.TotalExecutions != 2 {
		t.Errorf("expected 2 executions, got %d", snap.TotalExecutions)
	}
}

func TestMetricsRecorder_SetAutoSave(t *testing.T) {
	recorder := goalcon.NewMetricsRecorder(nil)
	recorder.SetAutoSave(false, 5)
	// Just verify it doesn't panic
}

func TestMetricsRecorder_ResetMetrics(t *testing.T) {
	recorder := goalcon.NewMetricsRecorder(nil)
	recorder.RecordOperatorExecution("op1", true, 100*time.Millisecond)

	if len(recorder.GetAllMetrics()) != 1 {
		t.Fatal("expected metrics before reset")
	}

	recorder.ResetMetrics()

	if len(recorder.GetAllMetrics()) != 0 {
		t.Fatal("expected empty metrics after reset")
	}
}

// mockMemoryStore for testing persistence
type mockMemoryStore struct {
	data map[string]map[string]*memory.MemoryRecord // [scope][key]record
}

func (m *mockMemoryStore) Remember(_ context.Context, key string, value map[string]interface{}, scope memory.MemoryScope) error {
	if m.data == nil {
		m.data = make(map[string]map[string]*memory.MemoryRecord)
	}
	if m.data[string(scope)] == nil {
		m.data[string(scope)] = make(map[string]*memory.MemoryRecord)
	}
	m.data[string(scope)][key] = &memory.MemoryRecord{
		Key:       key,
		Value:     value,
		Scope:     scope,
		Timestamp: time.Now(),
	}
	return nil
}

func (m *mockMemoryStore) Recall(_ context.Context, key string, scope memory.MemoryScope) (*memory.MemoryRecord, bool, error) {
	if m.data == nil || m.data[string(scope)] == nil {
		return nil, false, nil
	}
	record, ok := m.data[string(scope)][key]
	return record, ok, nil
}

func (m *mockMemoryStore) Search(_ context.Context, _ string, _ memory.MemoryScope) ([]memory.MemoryRecord, error) {
	return nil, nil
}

func (m *mockMemoryStore) Forget(_ context.Context, key string, scope memory.MemoryScope) error {
	if m.data != nil && m.data[string(scope)] != nil {
		delete(m.data[string(scope)], key)
	}
	return nil
}

func (m *mockMemoryStore) Summarize(_ context.Context, _ memory.MemoryScope) (string, error) {
	return "", nil
}

func TestSaveAndLoadMetricsFromMemory(t *testing.T) {
	store := &mockMemoryStore{data: make(map[string]map[string]*memory.MemoryRecord)}

	// Create and record metrics
	collection := make(goalcon.OperatorMetricsCollection)
	m1 := collection.GetOrCreateMetrics("op1")
	m1.RecordExecution(true, 100*time.Millisecond)
	m1.RecordExecution(false, 50*time.Millisecond)

	// Save to memory
	if err := goalcon.SaveMetricsToMemory(store, collection); err != nil {
		t.Fatalf("SaveMetricsToMemory failed: %v", err)
	}

	// Load from memory
	loaded := goalcon.LoadMetricsFromMemory(store)

	if len(loaded) != 1 {
		t.Errorf("expected 1 operator after load, got %d", len(loaded))
	}

	m1Loaded := loaded["op1"]
	if m1Loaded == nil {
		t.Fatal("expected op1 after load")
	}

	if m1Loaded.SuccessCount != 1 || m1Loaded.FailureCount != 1 {
		t.Errorf("expected 1 success and 1 failure, got %d successes and %d failures",
			m1Loaded.SuccessCount, m1Loaded.FailureCount)
	}
}

func TestMetricsJSON_Marshaling(t *testing.T) {
	m := &goalcon.OperatorMetrics{
		Name:            "TestOp",
		SuccessCount:    5,
		FailureCount:    2,
		TotalExecutions: 7,
		TotalDuration:   700 * time.Millisecond,
		AvgDuration:     100 * time.Millisecond,
		SuccessRate:     5.0 / 7.0,
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal back
	m2 := &goalcon.OperatorMetrics{}
	if err := json.Unmarshal(jsonBytes, m2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if m2.Name != m.Name || m2.SuccessCount != m.SuccessCount {
		t.Error("JSON round-trip failed")
	}
}

func TestMetricsRecorder_WithMemoryStore(t *testing.T) {
	store := &mockMemoryStore{data: make(map[string]map[string]*memory.MemoryRecord)}

	recorder := goalcon.NewMetricsRecorder(store)
	recorder.SetAutoSave(true, 2) // Save every 2 recordings

	// Record executions
	recorder.RecordOperatorExecution("op1", true, 100*time.Millisecond)
	recorder.RecordOperatorExecution("op2", true, 50*time.Millisecond)

	// At this point, auto-save should have triggered
	saved, exists, err := store.Recall(context.Background(), "goalcon.operator_metrics", memory.MemoryScopeProject)
	if err != nil || !exists || saved == nil {
		t.Fatal("expected metrics to be saved after auto-save interval")
	}

	// Load in new recorder instance
	recorder2 := goalcon.NewMetricsRecorder(store)
	if err := recorder2.LoadExisting(); err != nil {
		t.Fatalf("LoadExisting failed: %v", err)
	}

	metrics := recorder2.GetAllMetrics()
	if len(metrics) != 2 {
		t.Errorf("expected 2 operators after load, got %d", len(metrics))
	}
}

func TestSolver_WithMetricsRanking(t *testing.T) {
	// Create recorder and operators with different success rates
	recorder := goalcon.NewMetricsRecorder(nil)

	// good_op has 100% success rate
	recorder.RecordOperatorExecution("good_op", true, 50*time.Millisecond)
	recorder.RecordOperatorExecution("good_op", true, 50*time.Millisecond)

	// bad_op has 0% success rate
	recorder.RecordOperatorExecution("bad_op", false, 100*time.Millisecond)
	recorder.RecordOperatorExecution("bad_op", false, 100*time.Millisecond)

	// Create registry with both operators
	registry := &goalcon.types.OperatorRegistry{}
	registry.Register(goalcon.types.Operator{
		Name:    "good_op",
		Effects: []goalcon.types.Predicate{"target"},
	})
	registry.Register(goalcon.types.Operator{
		Name:    "bad_op",
		Effects: []goalcon.types.Predicate{"target"},
	})

	// Create solver with metrics
	solver := &goalcon.Solver{
		Operators: registry,
		MaxDepth:  10,
		Recorder:  recorder,
	}

	// Solve a goal
	goal := goalcon.types.GoalCondition{Predicates: []goalcon.types.Predicate{"target"}}
	result := solver.Solve(goal, goalcon.types.NewWorldState())

	// Should prefer good_op over bad_op
	if len(result.Plan.Steps) > 0 && result.Plan.Steps[0].Tool != "good_op" {
		t.Errorf("expected good_op to be selected, got %s", result.Plan.Steps[0].Tool)
	}
}
