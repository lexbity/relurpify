package features

import (
	"context"
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

// TestOperatorMetricsSnapshot verifies snapshot structure.
func TestOperatorMetricsSnapshot(t *testing.T) {
	snapshot := &OperatorMetricsSnapshot{
		OperatorName:      "analyze",
		TotalRuns:         10,
		SuccessfulRuns:    8,
		FailedRuns:        2,
		AverageDuration:   5,
		MinDuration:       2,
		MaxDuration:       8,
		SuccessRate:       0.8,
		MostCommonCost:    CostClassMedium,
		MostCommonRetry:   RetryClassIdempotent,
		LastObservedError: "timeout",
	}

	if snapshot.TotalRuns != 10 {
		t.Errorf("Expected 10 total runs, got %d", snapshot.TotalRuns)
	}
	if snapshot.SuccessRate != 0.8 {
		t.Errorf("Expected 0.8 success rate, got %f", snapshot.SuccessRate)
	}
	if snapshot.MostCommonCost != CostClassMedium {
		t.Errorf("Expected medium cost, got %s", snapshot.MostCommonCost)
	}
}

// TestOutputValidationSchema verifies schema structure.
func TestOutputValidationSchema(t *testing.T) {
	schema := &OutputValidationSchema{
		ExpectedKeys:  []string{"code", "error"},
		OptionalKeys:  []string{"warning"},
		Schema:        map[string]any{"type": "object"},
		MinOutputSize: 2,
		MaxOutputSize: 10,
	}

	if len(schema.ExpectedKeys) != 2 {
		t.Errorf("Expected 2 expected keys, got %d", len(schema.ExpectedKeys))
	}
	if schema.MinOutputSize != 2 {
		t.Errorf("Expected min size 2, got %d", schema.MinOutputSize)
	}
}

// TestValidateOutputAgainstSchema validates output against schema.
func TestValidateOutputAgainstSchema(t *testing.T) {
	schema := &OutputValidationSchema{
		ExpectedKeys:  []string{"result", "status"},
		MinOutputSize: 2,
		MaxOutputSize: 5,
	}

	// Valid output
	validOutput := map[string]any{
		"result": "success",
		"status": "done",
	}
	valid, issues := ValidateOutputAgainstSchema(validOutput, schema)
	if !valid {
		t.Errorf("Expected valid output, got issues: %v", issues)
	}

	// Missing required key
	invalidOutput := map[string]any{
		"result": "success",
	}
	valid, issues = ValidateOutputAgainstSchema(invalidOutput, schema)
	if valid {
		t.Error("Expected invalid output for missing required key")
	}
	if len(issues) == 0 {
		t.Error("Expected issues for missing key")
	}

	// Output too small
	tinyOutput := map[string]any{
		"result": "success",
	}
	valid, issues = ValidateOutputAgainstSchema(tinyOutput, schema)
	if valid {
		t.Error("Expected invalid output for size below minimum")
	}

	// Output too large
	largeOutput := map[string]any{
		"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6,
	}
	valid, issues = ValidateOutputAgainstSchema(largeOutput, schema)
	if valid {
		t.Error("Expected invalid output for size above maximum")
	}
}

// TestValidateOutputWithNilOutput handles nil output.
func TestValidateOutputWithNilOutput(t *testing.T) {
	schema := &OutputValidationSchema{
		ExpectedKeys: []string{"result"},
	}

	valid, issues := ValidateOutputAgainstSchema(nil, schema)
	if valid {
		t.Error("Expected invalid for nil output")
	}
	if len(issues) == 0 {
		t.Error("Expected issues for nil output")
	}
}

// TestSchedulingHint verifies hint structure.
func TestSchedulingHint(t *testing.T) {
	hint := SchedulingHint{
		StepID:            "step_1",
		RecommendedOrder:  1,
		CanParallelize:    true,
		EstimatedDuration: 5,
		CostOptimization:  "fast_first",
	}

	if hint.StepID != "step_1" {
		t.Errorf("Expected step_1, got %s", hint.StepID)
	}
	if hint.RecommendedOrder != 1 {
		t.Errorf("Expected order 1, got %d", hint.RecommendedOrder)
	}
	if !hint.CanParallelize {
		t.Error("CanParallelize should be true")
	}
}

// TestOptimizeStepOrderingWithCosts generates scheduling hints.
func TestOptimizeStepOrderingWithCosts(t *testing.T) {
	plan := &core.Plan{
		Goal: "test",
		Steps: []core.PlanStep{
			{ID: "step_fast", Tool: "fast_op", Description: "Fast step"},
			{ID: "step_slow", Tool: "slow_op", Description: "Slow step"},
			{ID: "step_medium", Tool: "medium_op", Description: "Medium step"},
		},
	}

	metadata := map[string]OperatorMetadata{
		"fast_op": {CostClass: CostClassFast},
		"slow_op": {CostClass: CostClassSlow},
		"medium_op": {CostClass: CostClassMedium},
	}

	hints := OptimizeStepOrdering(plan, metadata)

	if len(hints) != 3 {
		t.Errorf("Expected 3 hints, got %d", len(hints))
	}

	// Verify ordering: fast (0), medium (1), slow (2)
	if len(hints) >= 1 && hints[0].StepID != "step_fast" {
		t.Errorf("Expected first step to be fast, got %s", hints[0].StepID)
	}
}

// TestOptimizeStepOrderingEmpty handles empty plan.
func TestOptimizeStepOrderingEmpty(t *testing.T) {
	plan := &core.Plan{
		Goal: "test",
	}

	hints := OptimizeStepOrdering(plan, nil)
	if len(hints) != 0 {
		t.Errorf("Expected 0 hints for empty plan, got %d", len(hints))
	}
}

// TestSortStepsByOptimization sorts steps by cost.
func TestSortStepsByOptimization(t *testing.T) {
	steps := []core.PlanStep{
		{ID: "step_slow", Tool: "slow_op"},
		{ID: "step_fast", Tool: "fast_op"},
		{ID: "step_medium", Tool: "medium_op"},
	}

	metadata := map[string]OperatorMetadata{
		"fast_op": {CostClass: CostClassFast},
		"slow_op": {CostClass: CostClassSlow},
		"medium_op": {CostClass: CostClassMedium},
	}

	sorted := SortStepsByOptimization(steps, metadata, "fast_first")

	if len(sorted) != 3 {
		t.Errorf("Expected 3 sorted steps, got %d", len(sorted))
	}

	// First should be fast
	if sorted[0].ID != "step_fast" {
		t.Errorf("Expected first step to be fast, got %s", sorted[0].ID)
	}

	// Last should be slow
	if sorted[2].ID != "step_slow" {
		t.Errorf("Expected last step to be slow, got %s", sorted[2].ID)
	}
}

// TestSortStepsByOptimizationSlowFirst sorts slow first.
func TestSortStepsByOptimizationSlowFirst(t *testing.T) {
	steps := []core.PlanStep{
		{ID: "step_fast", Tool: "fast_op"},
		{ID: "step_slow", Tool: "slow_op"},
	}

	metadata := map[string]OperatorMetadata{
		"fast_op": {CostClass: CostClassFast},
		"slow_op": {CostClass: CostClassSlow},
	}

	sorted := SortStepsByOptimization(steps, metadata, "slow_first")

	if sorted[0].ID != "step_slow" {
		t.Errorf("Expected slow first, got %s", sorted[0].ID)
	}
}

// TestExtractOutputValidationSchema extracts schema from metadata.
func TestExtractOutputValidationSchema(t *testing.T) {
	expectedOutput := map[string]any{
		"fields": []string{"code", "error"},
		"type": "object",
		"min_size": 2.0,
		"max_size": 10.0,
	}

	schema := ExtractOutputValidationSchema(expectedOutput)

	if schema == nil {
		t.Error("Expected non-nil schema")
	}
	if len(schema.ExpectedKeys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(schema.ExpectedKeys))
	}
	if schema.MinOutputSize != 2 {
		t.Errorf("Expected min size 2, got %d", schema.MinOutputSize)
	}
	if schema.MaxOutputSize != 10 {
		t.Errorf("Expected max size 10, got %d", schema.MaxOutputSize)
	}
}

// TestExtractOutputValidationSchemaWithNil handles nil input.
func TestExtractOutputValidationSchemaWithNil(t *testing.T) {
	schema := ExtractOutputValidationSchema(nil)
	if schema != nil {
		t.Error("Expected nil schema for nil input")
	}
}

// TestComputeEstimatedExecutionTime calculates total time.
func TestComputeEstimatedExecutionTime(t *testing.T) {
	plan := &core.Plan{
		Goal: "test",
		Steps: []core.PlanStep{
			{ID: "step_fast", Tool: "fast_op"},
			{ID: "step_slow", Tool: "slow_op"},
		},
	}

	metadata := map[string]OperatorMetadata{
		"fast_op": {CostClass: CostClassFast},
		"slow_op": {CostClass: CostClassSlow},
	}

	totalTime := ComputeEstimatedExecutionTime(plan, metadata)

	// Fast (1) + Slow (30) = 31
	expected := 31
	if totalTime != expected {
		t.Errorf("Expected total time %d, got %d", expected, totalTime)
	}
}

// TestComputeEstimatedExecutionTimeEmpty handles empty plan.
func TestComputeEstimatedExecutionTimeEmpty(t *testing.T) {
	plan := &core.Plan{Goal: "test"}
	totalTime := ComputeEstimatedExecutionTime(plan, nil)

	if totalTime != 0 {
		t.Errorf("Expected 0 for empty plan, got %d", totalTime)
	}
}

// TestShouldRetryStep determines retry eligibility.
func TestShouldRetryStep(t *testing.T) {
	tests := []struct {
		name           string
		retryClass     RetryClass
		attemptCount   int
		maxAttempts    int
		expectedRetry  bool
	}{
		{"No retry class", RetryClassNone, 0, 3, false},
		{"Idempotent first attempt", RetryClassIdempotent, 0, 3, true},
		{"Idempotent second attempt", RetryClassIdempotent, 1, 3, true},
		{"Max attempts exceeded", RetryClassIdempotent, 3, 3, false},
		{"Stateless retry", RetryClassStateless, 0, 3, true},
		{"Probed retry", RetryClassProbed, 0, 3, true},
		{"Unknown retry class", RetryClassUnknown, 0, 3, false},
	}

	for _, test := range tests {
		result := ShouldRetryStep("step_1", test.retryClass, nil, test.attemptCount, test.maxAttempts)
		if result != test.expectedRetry {
			t.Errorf("%s: expected %v, got %v", test.name, test.expectedRetry, result)
		}
	}
}

// TestEnrichExecutionContextWithHistoricalDataNilStore handles nil store.
func TestEnrichExecutionContextWithHistoricalDataNilStore(t *testing.T) {
	agent := &HTNAgent{}
	state := core.NewContext()

	// Should not panic with nil store
	agent.EnrichExecutionContextWithHistoricalData(context.Background(), state, "operator", nil)

	// No error should occur
}

// TestEnrichExecutionContextWithHistoricalDataNilState handles nil state.
func TestEnrichExecutionContextWithHistoricalDataNilState(t *testing.T) {
	agent := &HTNAgent{}

	// Should not panic with nil state
	agent.EnrichExecutionContextWithHistoricalData(context.Background(), nil, "operator", nil)

	// No error should occur
}

// TestRetrieveRelevantKnowledgeNilStore handles nil store.
func TestRetrieveRelevantKnowledgeNilStore(t *testing.T) {
	query := &KnowledgeQuery{
		MethodName: "test",
	}

	results := RetrieveRelevantKnowledge(context.Background(), nil, query)
	if results != nil {
		t.Error("Expected nil results for nil store")
	}
}

// TestRetrieveRelevantKnowledgeNilQuery handles nil query.
func TestRetrieveRelevantKnowledgeNilQuery(t *testing.T) {
	// Just pass nil store to test nil query handling
	results := RetrieveRelevantKnowledge(context.Background(), nil, nil)
	if results != nil {
		t.Error("Expected nil results for nil query")
	}
}

// TestCostToValue converts cost class to numeric.
func TestCostToValue(t *testing.T) {
	tests := []struct {
		cost     CostClass
		expected int
	}{
		{CostClassFast, 1},
		{CostClassMedium, 2},
		{CostClassSlow, 3},
		{CostClassUnknown, 2},
	}

	for _, test := range tests {
		result := costToValue(test.cost)
		if result != test.expected {
			t.Errorf("Expected %d for %s, got %d", test.expected, test.cost, result)
		}
	}
}

// TestGetCostClassForStep extracts cost from metadata.
func TestGetCostClassForStep(t *testing.T) {
	step := core.PlanStep{ID: "step_1", Tool: "analyze"}
	metadata := map[string]OperatorMetadata{
		"analyze": {CostClass: CostClassMedium},
	}

	cost := getCostClassForStep(step, metadata)
	if cost != CostClassMedium {
		t.Errorf("Expected medium cost, got %s", cost)
	}
}

// TestGetCostClassForStepMissing handles missing metadata.
func TestGetCostClassForStepMissing(t *testing.T) {
	step := core.PlanStep{ID: "step_1", Tool: "unknown"}
	metadata := map[string]OperatorMetadata{}

	cost := getCostClassForStep(step, metadata)
	if cost != CostClassUnknown {
		t.Errorf("Expected unknown cost, got %s", cost)
	}
}
