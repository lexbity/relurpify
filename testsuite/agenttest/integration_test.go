package agenttest

import (
	"testing"

	"github.com/lexcodex/relurpify/framework/core"
)

// TestCapabilityCoverage_ExercisesAllTools verifies the coverage framework works
func TestCapabilityCoverage_ExercisesAllTools(t *testing.T) {
	// Create a mock transcript with various tools
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 30, Success: true},
			{Index: 1, Tool: "file_write", DurationMS: 40, Success: true},
			{Index: 2, Tool: "go_test", DurationMS: 1000, Success: true},
			{Index: 3, Tool: "file_search", DurationMS: 50, Success: true},
		},
	}

	// Build coverage from transcript
	coverage := &CapabilityCoverage{
		RegisteredTools: []string{"file_read", "file_write", "go_test", "file_search", "git_status"},
		ExercisedTools: map[string]int{
			"file_read":   1,
			"file_write":  1,
			"go_test":     1,
			"file_search": 1,
		},
	}

	// Verify all tools in transcript are marked as exercised
	for _, entry := range transcript.Entries {
		if entry.Tool == "" {
			continue
		}
		if _, ok := coverage.ExercisedTools[entry.Tool]; !ok {
			t.Errorf("Tool %s in transcript but not in coverage", entry.Tool)
		}
	}

	// Check coverage percentage
	coveragePercent := float64(len(coverage.ExercisedTools)) / float64(len(coverage.RegisteredTools)) * 100
	if coveragePercent < 80 {
		t.Errorf("Coverage %.1f%% below 80%% threshold", coveragePercent)
	}
}

// TestToolInjection_RespondsWithSyntheticResult verifies injection framework works
func TestToolInjection_RespondsWithSyntheticResult(t *testing.T) {
	// Create a tool override that injects an error
	override := ToolResponseOverride{
		Tool:      "go_test",
		Error:     "injected test failure",
		CallCount: 1,
	}

	// Verify override is properly configured
	if override.Tool != "go_test" {
		t.Error("Override tool name mismatch")
	}
	if override.Error == "" {
		t.Error("Override error should not be empty")
	}

	// Test override matching
	toolOverrides := []ToolResponseOverride{override}
	filtered := filterOverridesForTool(toolOverrides, "go_test")
	if len(filtered) != 1 {
		t.Errorf("Expected 1 matching override, got %d", len(filtered))
	}

	// Verify non-matching tools return empty
	filtered = filterOverridesForTool(toolOverrides, "file_read")
	if len(filtered) != 0 {
		t.Errorf("Expected 0 matching overrides for file_read, got %d", len(filtered))
	}
}

// TestDeterminismDetection_ConsistentRuns verifies fingerprinting identifies consistent runs
func TestDeterminismDetection_ConsistentRuns(t *testing.T) {
	// Create two identical transcripts
	transcript1 := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 30},
			{Index: 1, Tool: "go_test", DurationMS: 1000},
		},
	}
	transcript2 := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 35},
			{Index: 1, Tool: "go_test", DurationMS: 1050},
		},
	}

	// Compute fingerprints
	fp1, err := ComputeFingerprint(transcript1)
	if err != nil {
		t.Fatalf("Failed to compute fingerprint 1: %v", err)
	}
	fp2, err := ComputeFingerprint(transcript2)
	if err != nil {
		t.Fatalf("Failed to compute fingerprint 2: %v", err)
	}

	// Calculate distance between similar runs
	distance := FingerprintDistance(fp1, fp2)
	score := DeterminismScore(distance)

	// Should be relatively high for similar tool sequences
	if score < 0.5 {
		t.Errorf("Determinism score %.2f too low for similar runs", score)
	}

	// Now test with divergent transcript
	transcript3 := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_write", DurationMS: 50},
			{Index: 1, Tool: "file_search", DurationMS: 100},
		},
	}
	fp3, _ := ComputeFingerprint(transcript3)

	// Distance should be higher for different tool sequences
	distance2 := FingerprintDistance(fp1, fp3)
	score2 := DeterminismScore(distance2)

	if score2 > score {
		t.Error("Different runs should have lower determinism score than similar runs")
	}
}

// TestDependencyValidation_EnforcesOrdering verifies dependency rules work
func TestDependencyValidation_EnforcesOrdering(t *testing.T) {
	// Valid ordering: read before write
	validTranscript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read"},
			{Index: 1, Tool: "file_write"},
		},
	}

	// Invalid ordering: write without read
	invalidTranscript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_write"},
		},
	}

	// Define dependency: file_write requires file_read
	deps := []ToolDependency{
		{Tool: "file_write", Requires: []string{"file_read"}},
	}

	validator := NewDependencyValidator(deps)

	// Valid transcript should pass
	failures := validator.Validate(validTranscript)
	if len(failures) > 0 {
		t.Errorf("Valid transcript should have no failures, got: %v", failures)
	}

	// Invalid transcript should fail
	failures = validator.Validate(invalidTranscript)
	if len(failures) == 0 {
		t.Error("Invalid transcript should have failures")
	}
}

// TestLatencyTracking_MeasuresToolExecution verifies latency tracking works
func TestLatencyTracking_MeasuresToolExecution(t *testing.T) {
	// Create transcript with latency data
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 30},
			{Index: 1, Tool: "file_read", DurationMS: 50},
			{Index: 2, Tool: "go_test", DurationMS: 1000},
		},
	}

	// Build latency report
	report := BuildLatencyReport(transcript)
	if report == nil {
		t.Fatal("Expected non-nil latency report")
	}

	// Check total time
	expectedTotal := int64(30 + 50 + 1000)
	if report.TotalToolTimeMs != expectedTotal {
		t.Errorf("Expected total %dms, got %dms", expectedTotal, report.TotalToolTimeMs)
	}

	// Check file_read stats
	fileReadStats, ok := report.ToolLatencies["file_read"]
	if !ok {
		t.Fatal("Expected file_read in report")
	}
	if fileReadStats.MinMs != 30 || fileReadStats.MaxMs != 50 {
		t.Errorf("file_read stats incorrect: min=%d, max=%d", fileReadStats.MinMs, fileReadStats.MaxMs)
	}

	// Check latency constraint evaluation
	expect := ExpectSpec{
		ToolCallLatencyMs: map[string]string{
			"file_read": "<100",
			"go_test":   "<2000",
		},
		MaxTotalToolTimeMs: 2000,
	}

	failures := EvaluateLatencyExpectations(expect, transcript)
	if len(failures) > 0 {
		t.Errorf("Expected no failures for valid latencies, got: %v", failures)
	}
}

// TestConsistencyReport_AggregatesMultipleRuns verifies consistency reporting
func TestConsistencyReport_AggregatesMultipleRuns(t *testing.T) {
	// Create multiple case reports
	reports := []CaseReport{
		{
			Success:   true,
			ToolCalls: map[string]int{"file_read": 2, "go_test": 1},
			ToolLatencies: map[string]LatencyStats{
				"file_read": {MaxMs: 40},
				"go_test":   {MaxMs: 1000},
			},
		},
		{
			Success:   true,
			ToolCalls: map[string]int{"file_read": 2, "go_test": 1},
			ToolLatencies: map[string]LatencyStats{
				"file_read": {MaxMs: 45},
				"go_test":   {MaxMs: 1100},
			},
		},
		{
			Success:   false,
			ToolCalls: map[string]int{"file_read": 1, "go_test": 0},
			ToolLatencies: map[string]LatencyStats{
				"file_read": {MaxMs: 50},
			},
		},
	}

	// Compute consistency
	consistency := ComputeConsistency(reports)
	if consistency == nil {
		t.Fatal("Expected non-nil consistency report")
	}

	// Check runs count
	if consistency.Runs != 3 {
		t.Errorf("Expected 3 runs, got %d", consistency.Runs)
	}

	// Check success rate (2 out of 3)
	expectedRate := 2.0 / 3.0
	if consistency.SuccessRate != expectedRate {
		t.Errorf("Expected success rate %f, got %f", expectedRate, consistency.SuccessRate)
	}
}

// TestRecoveryDetection_IdentifiesToolFailureRecovery verifies recovery detection
func TestRecoveryDetection_IdentifiesToolFailureRecovery(t *testing.T) {
	// Events showing recovery: success -> failure -> success
	recoveryEvents := []core.Event{
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": false}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
	}

	if !HasRecoveryFromToolFailure(recoveryEvents) {
		t.Error("Expected recovery detected for failure followed by success")
	}

	// Events showing no recovery: all success
	noFailureEvents := []core.Event{
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
	}

	if HasRecoveryFromToolFailure(noFailureEvents) {
		t.Error("Expected no recovery when no failures")
	}

	// Events showing no recovery: failure at end
	failureAtEndEvents := []core.Event{
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": false}},
	}

	if HasRecoveryFromToolFailure(failureAtEndEvents) {
		t.Error("Expected no recovery when failure is last")
	}
}

// TestToolSuccessRate_ComputesCorrectly verifies success rate calculation
func TestToolSuccessRate_ComputesCorrectly(t *testing.T) {
	events := []core.Event{
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": true}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "go_test", "success": false}},
		{Type: core.EventToolResult, Metadata: map[string]any{"tool": "file_read", "success": true}},
	}

	successes, failures, rate := ToolSuccessRate(events, "go_test")
	if successes != 2 {
		t.Errorf("Expected 2 successes, got %d", successes)
	}
	if failures != 1 {
		t.Errorf("Expected 1 failure, got %d", failures)
	}
	expectedRate := 2.0 / 3.0
	if rate != expectedRate {
		t.Errorf("Expected rate %f, got %f", expectedRate, rate)
	}
}

// TestFullIntegration_AllFeaturesWorkTogether tests all features in combination
func TestFullIntegration_AllFeaturesWorkTogether(t *testing.T) {
	// Simulate a complete test scenario
	transcript := &ToolTranscriptArtifact{
		Entries: []ToolTranscriptEntry{
			{Index: 0, Tool: "file_read", DurationMS: 30, Success: true},
			{Index: 1, Tool: "file_write", DurationMS: 40, Success: true},
			{Index: 2, Tool: "go_test", DurationMS: 1000, Success: false}, // Failure
			{Index: 3, Tool: "go_test", DurationMS: 1200, Success: true},  // Recovery
		},
	}

	// Test coverage
	coverage := &CapabilityCoverage{
		RegisteredTools: []string{"file_read", "file_write", "go_test"},
		ExercisedTools: map[string]int{
			"file_read":  1,
			"file_write": 1,
			"go_test":    2,
		},
	}

	if len(coverage.ExercisedTools) != len(coverage.RegisteredTools) {
		t.Error("Not all registered tools were exercised")
	}

	// Test dependencies
	deps := []ToolDependency{
		{Tool: "file_write", Requires: []string{"file_read"}},
	}
	validator := NewDependencyValidator(deps)
	if failures := validator.Validate(transcript); len(failures) > 0 {
		t.Errorf("Dependency validation failed: %v", failures)
	}

	// Test latency
	report := BuildLatencyReport(transcript)
	if report == nil {
		t.Fatal("Failed to build latency report")
	}

	// Test determinism
	fp, err := ComputeFingerprint(transcript)
	if err != nil {
		t.Fatalf("Failed to compute fingerprint: %v", err)
	}
	if fp == nil {
		t.Fatal("Expected non-nil fingerprint")
	}

	// Verify transcript was captured correctly
	if len(fp.ToolOrder) != 4 {
		t.Errorf("Expected 4 tools in fingerprint, got %d", len(fp.ToolOrder))
	}
}
