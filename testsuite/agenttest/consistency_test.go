package agenttest

import (
	"testing"
)

func TestComputeConsistency(t *testing.T) {
	reports := []CaseReport{
		{Success: true, ToolCalls: map[string]int{"tool1": 2, "tool2": 3}},
		{Success: true, ToolCalls: map[string]int{"tool1": 2, "tool2": 3}},
		{Success: false, ToolCalls: map[string]int{"tool1": 2, "tool2": 4}},
	}

	report := ComputeConsistency(reports)

	if report == nil {
		t.Fatal("Expected non-nil report")
	}

	if report.Runs != 3 {
		t.Errorf("Expected 3 runs, got %d", report.Runs)
	}

	// 2 out of 3 succeeded
	expectedRate := 2.0 / 3.0
	if report.SuccessRate != expectedRate {
		t.Errorf("Expected success rate %f, got %f", expectedRate, report.SuccessRate)
	}

	// Tool count variance should be > 0 (due to different counts in run 3)
	if report.ToolCountVariance == 0 {
		t.Error("Expected non-zero tool count variance")
	}
}

func TestComputeConsistency_Empty(t *testing.T) {
	report := ComputeConsistency([]CaseReport{})
	if report != nil {
		t.Error("Expected nil for empty reports")
	}
}

func TestComputeConsistencyWithFingerprints(t *testing.T) {
	reports := []CaseReport{
		{Success: true},
		{Success: true},
	}

	fingerprints := []*ToolSequenceFingerprint{
		{ToolOrder: []string{"a", "b"}},
		{ToolOrder: []string{"a", "b"}},
	}

	report := ComputeConsistencyWithFingerprints(reports, fingerprints)

	if report == nil {
		t.Fatal("Expected non-nil report")
	}

	// Identical fingerprints should have distance 0
	if report.FingerprintDistance != 0.0 {
		t.Errorf("Expected distance 0.0 for identical fingerprints, got %f", report.FingerprintDistance)
	}

	// Score should be 1.0
	if report.DeterminismScore != 1.0 {
		t.Errorf("Expected determinism score 1.0, got %f", report.DeterminismScore)
	}
}

func TestAnalyzeToolCallVariance(t *testing.T) {
	reports := []CaseReport{
		{ToolCalls: map[string]int{"tool1": 2, "tool2": 3}}, // total: 5
		{ToolCalls: map[string]int{"tool1": 2, "tool2": 3}}, // total: 5
		{ToolCalls: map[string]int{"tool1": 3, "tool2": 4}}, // total: 7
	}

	variance := AnalyzeToolCallVariance(reports)

	if variance.Mean != (5.0+5.0+7.0)/3.0 {
		t.Errorf("Expected mean 5.667, got %f", variance.Mean)
	}

	if variance.Min != 5 {
		t.Errorf("Expected min 5, got %d", variance.Min)
	}

	if variance.Max != 7 {
		t.Errorf("Expected max 7, got %d", variance.Max)
	}

	if variance.Range != 2 {
		t.Errorf("Expected range 2, got %d", variance.Range)
	}

	// Should have some variance
	if variance.Variance == 0 {
		t.Error("Expected non-zero variance")
	}

	if variance.StdDev == 0 {
		t.Error("Expected non-zero std dev")
	}
}

func TestAnalyzeToolCallVariance_Empty(t *testing.T) {
	variance := AnalyzeToolCallVariance([]CaseReport{})
	if variance.Mean != 0 || variance.Variance != 0 {
		t.Error("Expected zero values for empty reports")
	}
}

func TestComputeVariance(t *testing.T) {
	tests := []struct {
		values []int
		want   float64
	}{
		{[]int{5, 5, 5}, 0.0},     // No variance
		{[]int{1, 2, 3}, 2.0/3.0}, // Some variance
		{[]int{}, 0.0},            // Empty
	}

	for _, tt := range tests {
		got := computeVariance(tt.values)
		if got != tt.want {
			t.Errorf("computeVariance(%v) = %f, want %f", tt.values, got, tt.want)
		}
	}
}

func TestIsDeterministic(t *testing.T) {
	tests := []struct {
		score     float64
		threshold string
		want      bool
	}{
		{0.95, ">0.9", true},
		{0.85, ">0.9", false},
		{0.90, ">=0.9", true},
		{0.89, ">=0.9", false},
		{0.8, "0.8", true},     // bare number means >=
		{0.7, "0.8", false},    // bare number means >=
		{0.0, ">0", false},    // edge case
		{0.01, ">0", true},     // edge case
	}

	for _, tt := range tests {
		got := IsDeterministic(tt.score, tt.threshold)
		if got != tt.want {
			t.Errorf("IsDeterministic(%f, %q) = %v, want %v", tt.score, tt.threshold, got, tt.want)
		}
	}
}

func TestFormatConsistencyReport(t *testing.T) {
	report := &ConsistencyReport{
		Runs:                5,
		SuccessRate:         0.8,
		ToolCountVariance:   2.5,
		FingerprintDistance: 0.1,
		DeterminismScore:    0.9,
	}

	formatted := FormatConsistencyReport(report)

	// Check that report contains expected sections
	expectedStrings := []string{
		"Consistency Report",
		"Runs: 5",
		"Success Rate: 80.0%",
		"Determinism Score: 90.00%",
		"Highly deterministic",
	}

	for _, expected := range expectedStrings {
		if !containsString(formatted, expected) {
			t.Errorf("Expected report to contain %q", expected)
		}
	}
}

func TestFormatConsistencyReport_Nil(t *testing.T) {
	formatted := FormatConsistencyReport(nil)
	if formatted != "Consistency Report: nil" {
		t.Errorf("Unexpected nil report format: %s", formatted)
	}
}

func TestFormatConsistencyReport_Interpretations(t *testing.T) {
	tests := []struct {
		score           float64
		expectedKeyword string
	}{
		{0.95, "Highly deterministic"},
		{0.85, "Moderately deterministic"},
		{0.60, "Low determinism"},
		{0.30, "Non-deterministic"},
	}

	for _, tt := range tests {
		report := &ConsistencyReport{
			Runs:             5,
			DeterminismScore: tt.score,
		}
		formatted := FormatConsistencyReport(report)
		if !containsString(formatted, tt.expectedKeyword) {
			t.Errorf("Expected report for score %f to contain %q", tt.score, tt.expectedKeyword)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || findSubstr(s, substr))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
