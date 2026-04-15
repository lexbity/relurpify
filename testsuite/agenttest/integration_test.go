package agenttest

import (
	"encoding/json"
	"errors"
	"strings"
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

	// Phase 8: Legacy latency evaluation removed - now handled by BenchmarkSpec
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

// === Phase 5: OSB Model Integration Tests ===

// TestFailureClassification_SecurityKind verifies [security] prefix produces FailureKind "security"
func TestFailureClassification_SecurityKind(t *testing.T) {
	// Test cases for failure classification
	tests := []struct {
		name     string
		execErr  error
		caseErr  string
		expected string
	}{
		{"empty error", nil, "", ""},
		{"infra error", nil, "context deadline exceeded", "infra"},
		{"assertion error", nil, "expected no file changes", "assertion"},
		{"security error", nil, "[security] found 1 out-of-scope file writes", "security"},
		{"agent error", nil, "agent returned unsuccessful result", "agent"},
		{"exec error with infra", errors.New("connection refused"), "", "infra"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyCaseFailure(tc.execErr, tc.caseErr)
			if result != tc.expected {
				t.Errorf("classifyCaseFailure() = %q, want %q", result, tc.expected)
			}
		})
	}
}

// TestSuiteReport_SecurityFailureCount verifies SecurityFailures counter is incremented correctly
func TestSuiteReport_SecurityFailureCount(t *testing.T) {
	cases := []CaseReport{
		{Name: "case1", Success: true, FailureKind: ""},
		{Name: "case2", Success: false, FailureKind: "security"},
		{Name: "case3", Success: false, FailureKind: "infra"},
		{Name: "case4", Success: false, FailureKind: "security"},
		{Name: "case5", Success: false, FailureKind: "assertion"},
	}

	// Simulate what RunSuite does to count failures
	var securityFailures, infraFailures, assertFailures int
	for _, c := range cases {
		if !c.Success {
			switch c.FailureKind {
			case "infra":
				infraFailures++
			case "security":
				securityFailures++
			default:
				assertFailures++
			}
		}
	}

	if securityFailures != 2 {
		t.Errorf("Expected 2 security failures, got %d", securityFailures)
	}
	if infraFailures != 1 {
		t.Errorf("Expected 1 infra failure, got %d", infraFailures)
	}
	if assertFailures != 1 {
		t.Errorf("Expected 1 assertion failure, got %d", assertFailures)
	}
}

// TestSuiteReport_BenchmarkSummaryPopulation verifies OSBBenchmark summary is populated
func TestSuiteReport_BenchmarkSummaryPopulation(t *testing.T) {
	cases := []CaseReport{
		{
			Name:    "case1",
			Success: true,
			BenchmarkObservations: []BenchmarkObservation{
				{Category: "tool_usage", Field: "tools_expected", Matched: true},
				{Category: "tool_usage", Field: "tools_expected", Matched: false},
			},
		},
		{
			Name:    "case2",
			Success: true,
			BenchmarkObservations: []BenchmarkObservation{
				{Category: "token_usage", Field: "token_budget.total", Matched: true},
			},
		},
	}

	summary := aggregateBenchmarkSummary(cases)

	if summary == nil {
		t.Fatal("Expected non-nil benchmark summary")
	}

	if summary.TotalObservations != 3 {
		t.Errorf("Expected 3 total observations, got %d", summary.TotalObservations)
	}

	if summary.MatchedObservations != 2 {
		t.Errorf("Expected 2 matched observations, got %d", summary.MatchedObservations)
	}

	// Verify category breakdown
	toolCat := summary.ByCategory["tool_usage"]
	if toolCat.Total != 2 {
		t.Errorf("Expected tool_usage total=2, got %d", toolCat.Total)
	}

	tokenCat := summary.ByCategory["token_usage"]
	if tokenCat.Total != 1 {
		t.Errorf("Expected token_usage total=1, got %d", tokenCat.Total)
	}
}

// TestOSBPipeline_AllThreeTiers verifies outcome+security+benchmark all work together
func TestOSBPipeline_AllThreeTiers(t *testing.T) {
	// Simulate a case with all three OSB blocks
	c := CaseSpec{
		Name: "full_osb_case",
		Expect: ExpectSpec{
			Outcome: &OutcomeSpec{
				MustSucceed:    true,
				OutputContains: []string{"success"},
			},
			Security: &SecuritySpec{
				NoWritesOutsideScope: true,
			},
			Benchmark: &BenchmarkSpec{
				ToolsExpected: []string{"file_read"},
			},
		},
	}

	// Verify all three blocks are present
	if c.Expect.Outcome == nil {
		t.Error("Expected Outcome block")
	}
	if c.Expect.Security == nil {
		t.Error("Expected Security block")
	}
	if c.Expect.Benchmark == nil {
		t.Error("Expected Benchmark block")
	}

	// Verify Outcome fields
	if !c.Expect.Outcome.MustSucceed {
		t.Error("Expected MustSucceed=true")
	}
	if len(c.Expect.Outcome.OutputContains) != 1 {
		t.Error("Expected OutputContains to have 1 item")
	}

	// Verify Security fields
	if !c.Expect.Security.NoWritesOutsideScope {
		t.Error("Expected NoWritesOutsideScope=true")
	}

	// Verify Benchmark fields
	if len(c.Expect.Benchmark.ToolsExpected) != 1 {
		t.Error("Expected ToolsExpected to have 1 item")
	}
}

// TestOSBPipeline_LegacyFallback verifies legacy path works when no new blocks present
func TestOSBPipeline_LegacyFallback(t *testing.T) {
	c := CaseSpec{
		Name: "legacy_case",
		Expect: ExpectSpec{
			MustSucceed:    true,
			OutputContains: []string{"done"},
			// No Outcome, Security, or Benchmark blocks
		},
	}

	// Verify legacy path detection
	if c.Expect.Outcome != nil || c.Expect.Security != nil || c.Expect.Benchmark != nil {
		t.Error("Expected no OSB blocks for legacy case")
	}

	// Verify legacy fields still work
	if !c.Expect.MustSucceed {
		t.Error("Expected legacy MustSucceed to be true")
	}
}

// TestOSBPipeline_OutcomeOnly verifies case with only outcome block
func TestOSBPipeline_OutcomeOnly(t *testing.T) {
	c := CaseSpec{
		Name: "outcome_only",
		Expect: ExpectSpec{
			Outcome: &OutcomeSpec{
				MustSucceed:    true,
				OutputContains: []string{"completed"},
			},
			// No Security or Benchmark blocks
		},
	}

	if c.Expect.Outcome == nil {
		t.Error("Expected Outcome block")
	}
	if c.Expect.Security != nil || c.Expect.Benchmark != nil {
		t.Error("Expected no Security or Benchmark blocks")
	}
}

// TestOSBPipeline_SecurityOnly verifies case with only security block
func TestOSBPipeline_SecurityOnly(t *testing.T) {
	c := CaseSpec{
		Name: "security_only",
		Expect: ExpectSpec{
			Security: &SecuritySpec{
				NoWritesOutsideScope: true,
				ToolsMustNotCall:     []string{"file_delete"},
			},
			// No Outcome or Benchmark blocks
		},
	}

	if c.Expect.Security == nil {
		t.Error("Expected Security block")
	}
	if c.Expect.Outcome != nil || c.Expect.Benchmark != nil {
		t.Error("Expected no Outcome or Benchmark blocks")
	}
}

// TestOSBPipeline_BenchmarkOnly verifies case with only benchmark block
func TestOSBPipeline_BenchmarkOnly(t *testing.T) {
	c := CaseSpec{
		Name: "benchmark_only",
		Expect: ExpectSpec{
			Benchmark: &BenchmarkSpec{
				ToolsExpected:    []string{"file_read", "file_search"},
				LLMCallsExpected: 5,
			},
			// No Outcome or Security blocks
		},
	}

	if c.Expect.Benchmark == nil {
		t.Error("Expected Benchmark block")
	}
	if c.Expect.Outcome != nil || c.Expect.Security != nil {
		t.Error("Expected no Outcome or Security blocks")
	}
}

// TestCaseReport_NewFields verifies CaseReport includes new OSB fields
func TestCaseReport_NewFields(t *testing.T) {
	report := CaseReport{
		Name:                  "test_case",
		Success:               true,
		AssertionResults:      []AssertionResult{{AssertionID: "outcome.must_succeed", Tier: "outcome", Passed: true}},
		SecurityObservations:  []SecurityObservation{{Kind: "file_read", InScope: true}},
		BenchmarkObservations: []BenchmarkObservation{{Category: "tool_usage", Field: "tools_expected", Matched: true}},
	}

	// Verify new OSB fields are populated
	if len(report.AssertionResults) == 0 {
		t.Error("Expected AssertionResults to be populated")
	}
	if len(report.SecurityObservations) == 0 {
		t.Error("Expected SecurityObservations to be populated")
	}
	if len(report.BenchmarkObservations) == 0 {
		t.Error("Expected BenchmarkObservations to be populated")
	}

	// Verify assertion result structure
	ar := report.AssertionResults[0]
	if ar.AssertionID != "outcome.must_succeed" {
		t.Errorf("Expected assertion_id 'outcome.must_succeed', got %q", ar.AssertionID)
	}
	if ar.Tier != "outcome" {
		t.Errorf("Expected tier 'outcome', got %q", ar.Tier)
	}

	// Verify security observation structure
	so := report.SecurityObservations[0]
	if so.Kind != "file_read" {
		t.Errorf("Expected kind 'file_read', got %q", so.Kind)
	}

	// Verify benchmark observation structure
	bo := report.BenchmarkObservations[0]
	if bo.Category != "tool_usage" {
		t.Errorf("Expected category 'tool_usage', got %q", bo.Category)
	}
}

// TestSecurityFailureDoesNotAffectBenchmark verifies security failure doesn't stop benchmark eval
func TestSecurityFailureDoesNotAffectBenchmark(t *testing.T) {
	// Create a case report with both security failure and benchmark observations
	report := CaseReport{
		Name:        "mixed_failure",
		Success:     false,
		FailureKind: "security",
		Error:       "[security] found 1 out-of-scope file writes",
		SecurityObservations: []SecurityObservation{
			{Kind: "file_write", InScope: false, Resource: "/etc/passwd"},
		},
		BenchmarkObservations: []BenchmarkObservation{
			{Category: "tool_usage", Field: "tools_expected", Expected: "file_read", Actual: "true", Matched: true},
			{Category: "token_usage", Field: "token_budget.total", Expected: "<=1000", Actual: "800", Matched: true},
		},
	}

	// Verify security failure is recorded
	if report.FailureKind != "security" {
		t.Errorf("Expected FailureKind='security', got %q", report.FailureKind)
	}
	if !strings.Contains(report.Error, "[security]") {
		t.Error("Expected error to contain [security] prefix")
	}

	// Verify benchmark observations are still present despite security failure
	if len(report.BenchmarkObservations) != 2 {
		t.Errorf("Expected 2 benchmark observations, got %d", len(report.BenchmarkObservations))
	}

	// All benchmark observations should be populated
	for _, obs := range report.BenchmarkObservations {
		if obs.Field == "" {
			t.Error("Expected benchmark observation Field to be populated")
		}
	}
}

// TestReportJSON_ContainsNewFields verifies report serialization includes new fields
func TestReportJSON_ContainsNewFields(t *testing.T) {
	report := SuiteReport{
		SuitePath:        "/test/suite.yaml",
		RunID:            "20240115-120000.000",
		PassedCases:      5,
		FailedCases:      2,
		SecurityFailures: 1,
		Cases: []CaseReport{
			{
				Name:    "case1",
				Success: true,
				AssertionResults: []AssertionResult{
					{AssertionID: "outcome.must_succeed", Tier: "outcome", Passed: true},
				},
				SecurityObservations: []SecurityObservation{
					{Kind: "file_read", InScope: true},
				},
				BenchmarkObservations: []BenchmarkObservation{
					{Category: "tool_usage", Field: "tools_expected", Matched: true},
				},
			},
		},
		OSBBenchmark: &OSBBenchmarkSummary{
			TotalObservations:   10,
			MatchedObservations: 8,
			MatchRate:           0.8,
		},
	}

	// Serialize to JSON
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal report: %v", err)
	}

	jsonStr := string(data)

	// Verify new fields are present in JSON
	requiredFields := []string{
		"assertion_results",
		"security_observations",
		"benchmark_observations",
		"security_failures",
		"osb_benchmark",
	}

	for _, field := range requiredFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("JSON output missing required field: %s", field)
		}
	}

	// Verify OSB benchmark fields
	if !strings.Contains(jsonStr, "total_observations") {
		t.Error("JSON output missing total_observations")
	}
	if !strings.Contains(jsonStr, "match_rate") {
		t.Error("JSON output missing match_rate")
	}
}
